package http2

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"

	utls "github.com/refraction-networking/utls"
	"golang.org/x/net/http2"
)

type TunnelClient struct {
	addr     string
	client   *http.Client
	username string
	password string
}

func makeUtlsConfig(t *tls.Config) *utls.Config {
	u := &utls.Config{
		ServerName: t.ServerName,
		MinVersion: t.MinVersion,
		RootCAs:    t.RootCAs,
		// ClientSessionCache: if using only one http2 connection, then the cache may not needed
	}
	for _, cert := range t.Certificates {
		ucert := utls.Certificate{
			Certificate:                 cert.Certificate,
			PrivateKey:                  cert.PrivateKey,
			OCSPStaple:                  cert.OCSPStaple,
			SignedCertificateTimestamps: cert.SignedCertificateTimestamps,
			Leaf:                        cert.Leaf,
		}
		for _, sign := range cert.SupportedSignatureAlgorithms {
			ucert.SupportedSignatureAlgorithms = append(ucert.SupportedSignatureAlgorithms, utls.SignatureScheme(sign))
		}
		u.Certificates = append(u.Certificates, ucert)
	}
	return u
}

// NewTunnelClient creates a client to issue CONNECT requests and tunnel traffic via HTTPS proxy.
func NewTunnelClient(addr string, cfg *tls.Config, username, password string) *TunnelClient {
	tc := &TunnelClient{
		addr:     addr,
		username: username,
		password: password,
	}

	roller, err := utls.NewRoller()
	if err != nil {
		panic(err)
	}

	// mitigate website fingerprinting via multiplexing of HTTP/2 ,
	// the fewer connections the better, so a single client is used here
	tc.client = &http.Client{Transport: &http2.Transport{
		TLSClientConfig: cfg,
		DialTLSContext: func(ctx context.Context, network, addr string, tlsCfg *tls.Config) (net.Conn, error) {
			// utls Roller does not accept IP address as server name
			if net.ParseIP(tlsCfg.ServerName) == nil {
				return roller.Dial(network, addr, tlsCfg.ServerName)
			}

			d := &net.Dialer{
				Timeout:   5 * time.Second,
				KeepAlive: 30 * time.Second,
			}
			conn, err := d.DialContext(ctx, network, addr)
			if err != nil {
				return nil, err
			}
			return utls.UClient(conn, makeUtlsConfig(tlsCfg), utls.HelloRandomized), nil
		},
		ReadIdleTimeout: 15 * time.Second,
		PingTimeout:     2 * time.Second,
	}}
	return tc
}

func (c *TunnelClient) DialContext(ctx context.Context, target string) (io.ReadWriteCloser, error) {
	pr, pw := io.Pipe()
	req := &http.Request{
		Method: http.MethodConnect,
		// For client requests, the URL's Host specifies the server to connect to,
		// while the Request's Host field optionally specifies the Host header
		// value to send in the HTTP request.
		URL:           &url.URL{Scheme: "https", Host: c.addr},
		Host:          target,
		Header:        make(http.Header),
		Body:          pr,
		ContentLength: -1,
	}
	req.Header.Set("User-Agent", "Chrome/115.0.0.0")
	if c.username != "" && c.password != "" {
		req.Header.Set("Proxy-Authorization",
			"Basic "+base64.StdEncoding.EncodeToString([]byte(c.username+":"+c.password)))
	}

	// the client return Responses from servers once
	// the response headers have been received
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, errors.New("http2 request: " + err.Error())
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, errors.New(resp.Status)
	}
	return &readWriteCloser{rc: resp.Body, wc: pw}, nil
}

func (c *TunnelClient) Close() error {
	c.client.CloseIdleConnections()
	return nil
}

func (c *TunnelClient) ConnNum() int {
	return -1
}

type readWriteCloser struct {
	rc io.ReadCloser
	wc *io.PipeWriter
}

func (rwc *readWriteCloser) Read(p []byte) (n int, err error) {
	return rwc.rc.Read(p)
}

func (rwc *readWriteCloser) Write(p []byte) (n int, err error) {
	return rwc.wc.Write(p)
}

func (rwc *readWriteCloser) Close() error {
	werr := rwc.wc.Close()
	rerr := rwc.rc.Close()
	if werr != nil {
		return werr
	}
	return rerr
}

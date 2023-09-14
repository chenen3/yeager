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

	"golang.org/x/net/http2"
)

type TunnelClient struct {
	addr     string
	client   *http.Client
	username string
	password string
}

// NewTunnelClient creates a client to issue CONNECT requests and tunnel traffic via HTTPS proxy.
func NewTunnelClient(addr string, cfg *tls.Config, username, password string) *TunnelClient {
	tc := &TunnelClient{
		addr:     addr,
		username: username,
		password: password,
	}

	// mitigate website fingerprinting via multiplexing of HTTP/2 ,
	// the fewer connections the better, so a single client is used here
	tc.client = &http.Client{Transport: &http2.Transport{
		TLSClientConfig: cfg,
		DialTLSContext: func(ctx context.Context, network, addr string, cfg *tls.Config) (net.Conn, error) {
			d := &net.Dialer{
				Timeout:   5 * time.Second,
				KeepAlive: 30 * time.Second,
			}
			return tls.DialWithDialer(d, "tcp", addr, cfg)
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
	return &stream{rc: resp.Body, wc: pw}, nil
}

func (c *TunnelClient) Close() error {
	c.client.CloseIdleConnections()
	return nil
}

type stream struct {
	rc io.ReadCloser
	wc *io.PipeWriter
}

func (s *stream) Read(p []byte) (n int, err error) {
	return s.rc.Read(p)
}

func (s *stream) Write(p []byte) (n int, err error) {
	return s.wc.Write(p)
}

func (s *stream) Close() error {
	werr := s.wc.Close()
	rerr := s.rc.Close()
	if werr != nil {
		return werr
	}
	return rerr
}

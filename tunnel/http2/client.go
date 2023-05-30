package http2

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"

	"golang.org/x/net/http2"
)

type TunnelClient struct {
	addr    string
	tlsConf *tls.Config
	client  *http.Client
}

func NewTunnelClient(addr string, tlsConf *tls.Config) *TunnelClient {
	tc := &TunnelClient{
		addr:    addr,
		tlsConf: tlsConf,
	}
	transport := &http2.Transport{
		DialTLSContext: func(ctx context.Context, network, addr string, _ *tls.Config) (net.Conn, error) {
			d := &net.Dialer{
				Timeout:   5 * time.Second,
				KeepAlive: 30 * time.Second,
			}
			return tls.DialWithDialer(d, "tcp", addr, tlsConf)
		},
	}
	tc.client = &http.Client{Transport: transport}
	return tc
}

func (c *TunnelClient) DialContext(ctx context.Context, target string) (io.ReadWriteCloser, error) {
	u, err := url.Parse("https://" + c.addr)
	if err != nil {
		return nil, err
	}
	pr, pw := io.Pipe()
	req := &http.Request{
		Method:        "CONNECT",
		URL:           &url.URL{Scheme: "https", Host: c.addr},
		Proto:         "HTTP/2",
		ProtoMajor:    2,
		ProtoMinor:    0,
		Header:        make(http.Header),
		Body:          pr,
		Host:          u.Host,
		ContentLength: -1,
	}
	req.Header.Add("dst", target)
	req.Header.Set("User-Agent", "Safari/605.1.15")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New(resp.Status)
	}
	return &rwc{resp.Body, pw}, nil
}

type rwc struct {
	rc io.ReadCloser
	wc io.WriteCloser
}

func (r *rwc) Read(p []byte) (n int, err error) {
	return r.rc.Read(p)
}

func (r *rwc) Write(p []byte) (n int, err error) {
	return r.wc.Write(p)
}

func (r *rwc) Close() error {
	we := r.wc.Close()
	re := r.rc.Close()
	if we != nil {
		return we
	}
	return re
}

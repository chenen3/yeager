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
	// To make the tunnel harder to detect, use as few connections as possible.
	tc.client = &http.Client{Transport: &http2.Transport{
		TLSClientConfig: tlsConf,
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

func (c *TunnelClient) DialContext(ctx context.Context, dst string) (io.ReadWriteCloser, error) {
	pr, pw := io.Pipe()
	req := &http.Request{
		Method:        http.MethodConnect,
		URL:           &url.URL{Scheme: "https", Host: c.addr},
		Header:        make(http.Header),
		Body:          pr,
		Host:          dst,
		ContentLength: -1,
	}
	req.Header.Set("User-Agent", "Chrome/115.0.0.0")

	// the client return Responses from servers once
	// the response headers have been received
	resp, err := c.client.Do(req)
	if err != nil {
		req.Body.Close()
		return nil, errors.New("http2 request: " + err.Error())
	}
	if resp.StatusCode != http.StatusOK {
		req.Body.Close()
		resp.Body.Close()
		return nil, errors.New(resp.Status)
	}

	rwc := &readWriteCloser{
		r: resp.Body,
		w: pw,
		onClose: func() {
			req.Body.Close()
			resp.Body.Close()
		},
	}
	return rwc, nil
}

func (c *TunnelClient) Close() error {
	c.client.CloseIdleConnections()
	return nil
}

func (c *TunnelClient) ConnNum() int {
	return -1
}

type readWriteCloser struct {
	r       io.Reader
	w       io.Writer
	onClose func()
}

func (rwc *readWriteCloser) Read(p []byte) (n int, err error) {
	return rwc.r.Read(p)
}

func (rwc *readWriteCloser) Write(p []byte) (n int, err error) {
	return rwc.w.Write(p)
}

func (rwc *readWriteCloser) Close() error {
	if rwc.onClose != nil {
		rwc.onClose()
	}
	return nil
}

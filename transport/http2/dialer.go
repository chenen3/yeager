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

	"github.com/chenen3/yeager/transport"
)

type streamDialer struct {
	addr     string
	username string
	password string
	client   *http.Client
}

var _ transport.StreamDialer = (*streamDialer)(nil)

// NewStreamDialer returns a transport.StreamDialer that
// connects to the specified HTTP2 server address
func NewStreamDialer(addr string, cfg *tls.Config, username, password string) *streamDialer {
	d := &streamDialer{
		addr:     addr,
		username: username,
		password: password,
	}

	// mitigate website fingerprinting via multiplexing of HTTP/2 ,
	// the fewer connections the better
	d.client = &http.Client{
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout:   5 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			TLSClientConfig:     cfg,
			ForceAttemptHTTP2:   true,
			MaxIdleConns:        100,
			IdleConnTimeout:     90 * time.Second,
			TLSHandshakeTimeout: 5 * time.Second,
		},
	}
	return d
}

func makeBasicAuth(username, password string) string {
	auth := username + ":" + password
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(auth))
}

func (c *streamDialer) Dial(ctx context.Context, target string) (transport.Stream, error) {
	pr, pw := io.Pipe()
	req := &http.Request{
		Method: http.MethodConnect,
		// For client requests, the URL's Host specifies the server to connect to
		URL:           &url.URL{Scheme: "https", Host: c.addr},
		Host:          target,
		Header:        make(http.Header),
		Body:          pr,
		ContentLength: -1,
	}
	req.Header.Set("User-Agent", "Chrome/115.0.0.0")
	if c.username != "" {
		req.Header.Set("Proxy-Authorization", makeBasicAuth(c.username, c.password))
	}

	// Once the client receives the header from server, it immediately returns a response
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, errors.New("http2 request: " + err.Error())
	}
	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		return nil, errors.New(resp.Status)
	}
	return &stream{rc: resp.Body, wc: pw}, nil
}

func (c *streamDialer) Close() error {
	c.client.CloseIdleConnections()
	return nil
}

type stream struct {
	rc io.ReadCloser
	wc *io.PipeWriter
}

func (b *stream) Read(p []byte) (n int, err error) {
	return b.rc.Read(p)
}

func (b *stream) Write(p []byte) (n int, err error) {
	return b.wc.Write(p)
}

func (b *stream) Close() error {
	werr := b.wc.Close()
	rerr := b.rc.Close()
	if werr != nil {
		return werr
	}
	return rerr
}

func (b *stream) CloseWrite() error {
	return b.wc.Close()
}

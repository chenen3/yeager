package http2

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"time"

	"github.com/chenen3/yeager/flow"
	"github.com/chenen3/yeager/transport"
)

type streamDialer struct {
	addr     string
	username string
	password string
	client   *http.Client
}

var _ transport.StreamDialer = (*streamDialer)(nil)

// NewStreamDialer creates a Transport.StreamDialer which issues CONNECT to HTTP/2 server.
// It compatible with Caddy's forward proxy.
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
				Timeout:   10 * time.Second,
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

func basicAuth(username, password string) string {
	auth := username + ":" + password
	return base64.StdEncoding.EncodeToString([]byte(auth))
}

func (d *streamDialer) Dial(ctx context.Context, target string) (transport.Stream, error) {
	pr, pw := io.Pipe()
	req, err := http.NewRequestWithContext(ctx, http.MethodConnect, "https://"+d.addr, pr)
	if err != nil {
		return nil, err
	}
	req.Host = target
	if d.username != "" {
		req.Header.Set("Proxy-Authorization", "Basic "+basicAuth(d.username, d.password))
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		dump, err := httputil.DumpResponse(resp, true)
		if err != nil {
			return nil, fmt.Errorf("failed to connect, status code: %s", resp.Status)
		}
		return nil, fmt.Errorf("failed to connect, response: %q", dump)
	}
	return &stream{writer: pw, reader: resp.Body}, nil
}

func (d *streamDialer) Close() error {
	d.client.CloseIdleConnections()
	return nil
}

type stream struct {
	writer *io.PipeWriter
	reader io.ReadCloser
}

func (s *stream) Read(p []byte) (n int, err error) {
	return s.reader.Read(p)
}

func (s *stream) Write(p []byte) (n int, err error) {
	return s.writer.Write(p)
}

func (s *stream) Close() error {
	s.writer.Close()
	return s.reader.Close()
}

func (s *stream) CloseWrite() error {
	return s.writer.Close()
}

func (s *stream) ReadFrom(r io.Reader) (n int64, err error) {
	return flow.Copy(s, r)
}

func (s *stream) WriteTo(w io.Writer) (written int64, err error) {
	return flow.Copy(w, s)
}

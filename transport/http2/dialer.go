package http2

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"sync"
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

// NewStreamDialer creates a transport.StreamDialer which issues CONNECT to HTTP/2 server.
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
	// logger.Debug.Printf("connected to %s", target)
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
	return bufferedCopy(s.writer, r)
}

func (s *stream) WriteTo(w io.Writer) (written int64, err error) {
	return bufferedCopy(w, s.reader)
}

var bufPool = sync.Pool{
	New: func() any {
		// refer to 16KB maxPlaintext in crypto/tls/common.go
		b := make([]byte, 16*1024)
		return &b
	},
}

// copy data from src to dst using buffer from pool
func bufferedCopy(dst io.Writer, src io.Reader) (written int64, err error) {
	buf := bufPool.Get().(*[]byte)
	for {
		nr, er := src.Read(*buf)
		if nr > 0 {
			nw, ew := dst.Write((*buf)[0:nr])
			if nw < 0 || nr < nw {
				nw = 0
				if ew == nil {
					ew = errors.New("invalid write result")
				}
			}
			written += int64(nw)
			if ew != nil {
				err = ew
				break
			}
			if nr != nw {
				err = io.ErrShortWrite
				break
			}
		}
		if er != nil {
			if er != io.EOF {
				err = er
			}
			break
		}
	}
	bufPool.Put(buf)
	return written, err
}

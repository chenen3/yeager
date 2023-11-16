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
	"net/url"
	"time"

	"github.com/chenen3/yeager/logger"
	"github.com/chenen3/yeager/transport"
)

type streamDialer struct {
	addr     string
	username string
	password string
	client   *http.Client

	quota      chan struct{}
	preStreams chan transport.Stream // pre-created streams
}

var _ transport.StreamDialer = (*streamDialer)(nil)

// NewStreamDialer returns a transport.StreamDialer that
// connects to the specified HTTP2 server address.
// The maxPreConnect parameter specifies the maximum number of streams that
// can create for future use. Given zero or negative value means do not pre-connect.
func NewStreamDialer(addr string, cfg *tls.Config, username, password string, maxPreConnect int) *streamDialer {
	d := &streamDialer{
		addr:     addr,
		username: username,
		password: password,
	}
	if maxPreConnect > 0 {
		d.quota = make(chan struct{}, maxPreConnect)
		d.preStreams = make(chan transport.Stream, maxPreConnect)
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

func basicAuth(username, password string) string {
	auth := username + ":" + password
	return base64.StdEncoding.EncodeToString([]byte(auth))
}

func (d *streamDialer) Dial(ctx context.Context, target string) (_ transport.Stream, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("h2: %s", err)
		}
	}()

	if cap(d.preStreams) == 0 {
		return d.connect(target)
	}

	// create stream for future use,
	// so subsequent callers don't have to wait hundreds of milliseconds.
	// To conserve resources, only do this when a quota is available.
	tryPreConnect := func() {
		select {
		case d.quota <- struct{}{}:
		default:
			return
		}
		stream, e := d.preConnect()
		if e != nil {
			<-d.quota
			logger.Error.Printf("h2 dial early: %s", e)
			return
		}
		d.preStreams <- stream
	}
	go tryPreConnect()

	select {
	case stream := <-d.preStreams:
		<-d.quota
		_, err = (&metadata{target}).WriteTo(stream)
		if err != nil {
			stream.Close()
			return nil, fmt.Errorf("send metadata: %s", err)
		}
		return stream, nil
	default:
		return d.connect(target)
	}
}

// connect to proxy server and returns a stream available for transport.
// It compatible with Caddy's forward proxy.
func (d *streamDialer) connect(target string) (transport.Stream, error) {
	pr, pw := io.Pipe()
	req := &http.Request{
		Method: http.MethodConnect,
		// For client requests, the URL's Host specifies the server to connect to
		URL:           &url.URL{Scheme: "https", Host: d.addr},
		Host:          target,
		Header:        make(http.Header),
		Body:          pr,
		ContentLength: -1,
	}
	req.Header.Set("User-Agent", "Chrome/115.0.0.0")
	if d.username != "" {
		req.Header.Set("Proxy-Authorization", "Basic "+basicAuth(d.username, d.password))
	}

	// the client returns a response immediately after receiving header from the server.
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

const pendingHost = "pending"

// preConnect connects to proxy server without sending target host in header,
// and returns a stream expecting that target host.
func (d *streamDialer) preConnect() (transport.Stream, error) {
	pr, pw := io.Pipe()
	req := &http.Request{
		Method: http.MethodConnect,
		// For client requests, the URL's Host specifies the server to connect to
		URL:           &url.URL{Scheme: "https", Host: d.addr},
		Host:          pendingHost,
		Header:        make(http.Header),
		Body:          pr,
		ContentLength: -1,
	}
	req.Header.Set("User-Agent", "Chrome/115.0.0.0")
	if d.username != "" {
		req.Header.Set("Proxy-Authorization", "Basic "+basicAuth(d.username, d.password))
	}

	// the client returns a response immediately after receiving header from the server.
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

type metadata struct {
	Hostport string
}

// [length, payload...]
func (md *metadata) WriteTo(w io.Writer) (int64, error) {
	size := len(md.Hostport)
	buf := make([]byte, 0, size+1)
	buf = append(buf, byte(size))
	buf = append(buf, []byte(md.Hostport)...)
	n, err := w.Write(buf)
	return int64(n), err
}

func (md *metadata) ReadFrom(r io.Reader) (int64, error) {
	var b [1]byte
	ns, err := io.ReadFull(r, b[:])
	if err != nil {
		return int64(ns), err
	}

	size := int(b[0])
	buf := make([]byte, size)
	n, err := io.ReadFull(r, buf)
	if err != nil {
		return int64(n), err
	}
	md.Hostport = string(buf)
	return int64(ns + n), nil
}

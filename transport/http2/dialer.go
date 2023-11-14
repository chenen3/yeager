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

	seats        chan struct{}
	earlyStreams chan transport.Stream
}

var _ transport.StreamDialer = (*streamDialer)(nil)

// NewStreamDialer returns a transport.StreamDialer that
// connects to the specified HTTP2 server address.
// The maxEarlyStream parameter specifies the maximum number of streams to dial early.
// This can save the caller a round trip time if a suitable value is given.
func NewStreamDialer(addr string, cfg *tls.Config, username, password string, maxEarlyStream int) *streamDialer {
	d := &streamDialer{
		addr:     addr,
		username: username,
		password: password,
	}
	if maxEarlyStream > 0 {
		d.seats = make(chan struct{}, maxEarlyStream)
		d.earlyStreams = make(chan transport.Stream, maxEarlyStream)
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

func (d *streamDialer) Dial(ctx context.Context, target string) (_ transport.Stream, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("h2: %s", err)
		}
	}()

	if cap(d.earlyStreams) == 0 {
		return d.dial(target)
	}

	// Dialing early so subsequent callers don't have to wait hundreds of milliseconds.
	// To avoid flooding the server, only dial after getting a seat
	tryDialEarly := func() {
		select {
		case d.seats <- struct{}{}:
		default:
			return
		}
		s, e := d.dialEarly()
		if e != nil {
			<-d.seats
			logger.Error.Printf("h2 dial early: %s", e)
			return
		}
		d.earlyStreams <- s
	}
	go tryDialEarly()

	var earlyStream transport.Stream
	select {
	case earlyStream = <-d.earlyStreams:
		<-d.seats
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
		return d.dial(target)
	}
	_, err = (&metadata{target}).WriteTo(earlyStream)
	if err != nil {
		earlyStream.Close()
		return nil, fmt.Errorf("send metadata: %s", err)
	}
	return earlyStream, nil
}

// dial connects to peer server and returns a stream available for transport.
// It compatible with Caddy's forward proxy.
func (d *streamDialer) dial(target string) (transport.Stream, error) {
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
		req.Header.Set("Proxy-Authorization", makeBasicAuth(d.username, d.password))
	}

	// the client returns a response immediately after receiving header from the server.
	resp, err := d.client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		return nil, errors.New(resp.Status)
	}
	return &stream{respBody: resp.Body, reqBodyWriter: pw}, nil
}

const pendingHost = "pending"

// dialEarly connects to peer server without sending target host in header,
// and returns a stream expecting the target host.
// It is an optimization for dialing.
func (d *streamDialer) dialEarly() (transport.Stream, error) {
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
		req.Header.Set("Proxy-Authorization", makeBasicAuth(d.username, d.password))
	}

	// the client returns a response immediately after receiving header from the server.
	resp, err := d.client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		return nil, errors.New(resp.Status)
	}
	return &stream{respBody: resp.Body, reqBodyWriter: pw}, nil
}

func (d *streamDialer) Close() error {
	d.client.CloseIdleConnections()
	return nil
}

type stream struct {
	respBody      io.ReadCloser
	reqBodyWriter *io.PipeWriter
}

func (s *stream) Read(p []byte) (n int, err error) {
	return s.respBody.Read(p)
}

func (s *stream) Write(p []byte) (n int, err error) {
	return s.reqBodyWriter.Write(p)
}

func (s *stream) Close() error {
	errReq := s.reqBodyWriter.Close()
	errResp := s.respBody.Close()
	if errReq != nil {
		return errReq
	}
	return errResp
}

func (s *stream) CloseWrite() error {
	return s.reqBodyWriter.Close()
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

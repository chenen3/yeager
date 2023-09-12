package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/chenen3/yeager/flow"
)

// refer to https://en.wikipedia.org/wiki/HTTP_tunnel
type httpProxy struct {
	mu         sync.Mutex
	lis        net.Listener
	activeConn map[net.Conn]struct{}
}

type dialFunc func(ctx context.Context, address string) (io.ReadWriteCloser, error)

var defaultDial = func(ctx context.Context, address string) (io.ReadWriteCloser, error) {
	var d net.Dialer
	return d.DialContext(ctx, "tcp", address)
}

// Serve serves connection accepted by lis,
// blocks until an unexpected error is encounttered or Close is called.
// If dial is nil, the net package's standard dialer is used.
func (s *httpProxy) Serve(lis net.Listener, dial dialFunc) error {
	s.mu.Lock()
	s.lis = lis
	s.mu.Unlock()
	if dial == nil {
		dial = defaultDial
	}
	for {
		conn, err := lis.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				err = nil
			}
			return err
		}

		s.trackConn(conn, true)
		go s.handleConn(conn, dial)
	}
}

func (s *httpProxy) handleConn(conn net.Conn, dial dialFunc) {
	defer s.trackConn(conn, false)
	defer conn.Close()

	conn.SetDeadline(time.Now().Add(5 * time.Second))
	dst, httpReq, err := handshake(conn)
	if err != nil {
		slog.Error(err.Error())
		return
	}
	conn.SetDeadline(time.Time{})

	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	stream, err := dial(ctx, dst)
	if err != nil {
		slog.Error(fmt.Sprintf("connect %s: %s", dst, err))
		return
	}
	defer stream.Close()

	if httpReq != nil {
		if err = httpReq.Write(stream); err != nil {
			slog.Error(err.Error())
			return
		}
	}

	err = flow.Relay(conn, stream)
	if err != nil && !canIgnore(err) {
		slog.Error(err.Error())
		return
	}
	slog.Debug("closed "+dst, durationKey, time.Since(start))
}

func (s *httpProxy) trackConn(c net.Conn, add bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.activeConn == nil {
		s.activeConn = make(map[net.Conn]struct{})
	}
	if add {
		s.activeConn[c] = struct{}{}
	} else {
		delete(s.activeConn, c)
	}
}

// Close close listener and all active connections
func (s *httpProxy) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var err error
	if s.lis != nil {
		err = s.lis.Close()
	}
	for c := range s.activeConn {
		c.Close()
		delete(s.activeConn, c)
	}
	return err
}

// handshake reads request from conn, returns host:port address and http request, if any
func handshake(conn net.Conn) (hostport string, httpReq *http.Request, err error) {
	var req *http.Request
	if req, err = http.ReadRequest(bufio.NewReader(conn)); err != nil {
		return "", nil, err
	}

	port := req.URL.Port()
	if req.Method == "CONNECT" {
		if port == "" {
			port = "443"
		}
		// reply https proxy request
		_, err = fmt.Fprintf(conn, "%s 200 Connection established\r\n\r\n", req.Proto)
		if err != nil {
			return "", nil, err
		}
	} else {
		if port == "" {
			port = "80"
		}
		// forward http proxy request
		httpReq = req
	}

	hostport = net.JoinHostPort(req.URL.Hostname(), port)
	return escape(hostport), httpReq, nil
}

// If unsanitized user input is written to a log entry,
// a malicious user may be able to forge new log entries.
// More detail see https://github.com/chenen3/yeager/security/code-scanning/15
func escape(s string) string {
	s = strings.ReplaceAll(s, "\n", "")
	return strings.ReplaceAll(s, "\r", "")
}

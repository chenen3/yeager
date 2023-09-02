package main

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/chenen3/yeager/flow"
)

// implements HTTP proxy server,
// refer to https://en.wikipedia.org/wiki/HTTP_tunnel
type httpProxy struct {
	mu         sync.Mutex
	lis        net.Listener
	activeConn map[net.Conn]struct{}
	done       chan struct{}
}

func newHTTPProxy() *httpProxy {
	s := &httpProxy{
		activeConn: make(map[net.Conn]struct{}),
		done:       make(chan struct{}),
	}
	return s
}

// Serve serves connection accepted by lis,
// blocks until an unexpected error is encounttered or Close is called
func (s *httpProxy) Serve(lis net.Listener, connect connectFunc) error {
	s.mu.Lock()
	s.lis = lis
	s.mu.Unlock()

	for {
		conn, err := lis.Accept()
		if err != nil {
			select {
			case <-s.done:
				return nil
			default:
				return err
			}
		}

		s.trackConn(conn, true)
		go s.handleConn(conn, connect)
	}
}

func (s *httpProxy) handleConn(conn net.Conn, connect connectFunc) {
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
	stream, err := connect(ctx, dst)
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
	if add {
		s.activeConn[c] = struct{}{}
	} else {
		delete(s.activeConn, c)
	}
}

// ConnNum returns the number of active connections
func (s *httpProxy) ConnNum() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.activeConn)
}

// Close close listener and all active connections
func (s *httpProxy) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.done != nil {
		close(s.done)
	}
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

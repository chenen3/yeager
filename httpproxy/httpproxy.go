// This package implements HTTP proxy server.
// Refer to https://en.wikipedia.org/wiki/HTTP_tunnel
// Any data sent to the proxy server will be forwarded, unmodified, to the remote host
package httpproxy

import (
	"bufio"
	"context"
	"errors"
	"expvar"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	ylog "github.com/chenen3/yeager/log"
	ynet "github.com/chenen3/yeager/net"
	"github.com/chenen3/yeager/tunnel"
)

// Server implement interface Service
type Server struct {
	mu          sync.Mutex
	lis         net.Listener
	activeConns map[net.Conn]struct{}
}

// Serve will return a non-nil error unless Close is called.
func (s *Server) Serve(lis net.Listener, d tunnel.Dialer) error {
	s.mu.Lock()
	s.lis = lis
	s.mu.Unlock()

	for {
		conn, err := lis.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				err = nil
			}
			return err
		}

		// tracking connection in handleConn synchronously will casue unnecessary blocking
		s.trackConn(conn, true)
		go s.handleConn(conn, d)
	}
}

func (s *Server) handleConn(conn net.Conn, d tunnel.Dialer) {
	defer s.trackConn(conn, false)
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(ynet.HandshakeTimeout))
	dst, httpReq, err := handshake(conn)
	conn.SetDeadline(time.Time{})
	if err != nil {
		log.Printf("handshake: %s", err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), ynet.DialTimeout)
	defer cancel()
	remote, err := d.DialContext(ctx, dst)
	if err != nil {
		log.Printf("dial %s: %s", dst, err)
		return
	}
	defer remote.Close()

	if httpReq != nil {
		if err := httpReq.Write(remote); err != nil {
			log.Print(err)
			return
		}
	}
	f := ynet.NewForwarder(conn, remote)
	go f.FromClient()
	go f.ToClient()
	if err := <-f.C; err != nil {
		ylog.Debugf("forward %s: %s", dst, err)
	}
}

var connCount = expvar.NewInt("httpProxyConnCount")

func (s *Server) trackConn(c net.Conn, add bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.activeConns == nil {
		s.activeConns = make(map[net.Conn]struct{})
	}
	if add {
		s.activeConns[c] = struct{}{}
		connCount.Add(1)
	} else {
		delete(s.activeConns, c)
		connCount.Add(-1)
	}
}

func (s *Server) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var err error
	if s.lis != nil {
		err = s.lis.Close()
	}
	for c := range s.activeConns {
		c.Close()
		delete(s.activeConns, c)
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

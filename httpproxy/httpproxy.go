// This package implements HTTP proxy server.
// Refer to https://en.wikipedia.org/wiki/HTTP_tunnel
// Any data sent to the proxy server will be forwarded, unmodified, to the remote host
package httpproxy

import (
	"bufio"
	"context"
	"expvar"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/chenen3/yeager/debug"
	ynet "github.com/chenen3/yeager/net"
	"github.com/chenen3/yeager/tunnel"
)

var connCount = new(debug.Counter)

func init() {
	expvar.Publish("connhttp", connCount)
}

type Server struct {
	mu         sync.Mutex
	lis        net.Listener
	activeConn map[net.Conn]struct{}
	done       chan struct{}
}

func NewServer() *Server {
	s := &Server{
		activeConn: make(map[net.Conn]struct{}),
		done:       make(chan struct{}),
	}
	connCount.Register(s.Len)
	return s
}

// Serve serves connection accepted by lis,
// blocks until an unexpected error is encounttered or Close is called
func (s *Server) Serve(lis net.Listener, d tunnel.Dialer) error {
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
		go s.handleConn(conn, d)
	}
}

func (s *Server) handleConn(conn net.Conn, d tunnel.Dialer) {
	defer s.trackConn(conn, false)
	defer conn.Close()

	conn.SetDeadline(time.Now().Add(ynet.HandshakeTimeout))
	dst, httpReq, err := handshake(conn)
	if err != nil {
		log.Printf("handshake: %s", err)
		return
	}
	conn.SetDeadline(time.Time{})

	ctx, cancel := context.WithTimeout(context.Background(), ynet.DialTimeout)
	defer cancel()
	remote, err := d.DialContext(ctx, dst)
	if err != nil {
		log.Printf("dial %s: %s", dst, err)
		return
	}
	defer remote.Close()

	if httpReq != nil {
		if err = httpReq.Write(remote); err != nil {
			log.Print(err)
			return
		}
	}

	sent, recv, err := ynet.Relay(conn, remote)
	if err != nil {
		log.Printf("relay %s: %s", dst, err)
		return
	}
	if debug.Enabled() {
		log.Printf("done %s, sent %s, recv %s", dst, ynet.ReadableBytes(sent), ynet.ReadableBytes(recv))
	}
}

func (s *Server) trackConn(c net.Conn, add bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if add {
		s.activeConn[c] = struct{}{}
	} else {
		delete(s.activeConn, c)
	}
}

// Len returns the number of active connections
func (s *Server) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.activeConn)
}

// Close close listener and all active connections
func (s *Server) Close() error {
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

package http

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/chenen3/yeager/log"
	"github.com/chenen3/yeager/proxy/common"
)

// Server implements the proxy.Inbounder interface
type Server struct {
	addr    string
	handler func(c net.Conn, addr string)
	lis     net.Listener

	mu         sync.Mutex
	activeConn map[net.Conn]struct{}
	done       chan struct{}
	ready      chan struct{} // imply that server is ready to accept connection, testing only
}

func NewServer(addr string) (*Server, error) {
	if addr == "" {
		return nil, errors.New("empty address")
	}

	return &Server{
		addr:       addr,
		ready:      make(chan struct{}),
		done:       make(chan struct{}),
		activeConn: make(map[net.Conn]struct{}),
	}, nil
}

func (s *Server) Handle(handler func(c net.Conn, addr string)) {
	s.handler = handler
}

func (s *Server) ListenAndServe() error {
	lis, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("http proxy failed to listen, err: %s", err)
	}
	s.lis = lis
	log.L().Infof("http proxy listening %s", s.addr)

	close(s.ready)
	for {
		conn, err := lis.Accept()
		if err != nil {
			select {
			case <-s.done:
				return nil
			default:
			}
			log.L().Warnf(err.Error())
			continue
		}

		go func(conn net.Conn) {
			s.trackConn(conn)
			defer s.untrackConn(conn)
			addr, reqcopy, err := s.handshake(conn)
			if err != nil {
				log.L().Errorf("handshake: %s", err.Error())
				conn.Close()
				return
			}

			// forward HTTP proxy request
			if len(reqcopy) > 0 {
				conn = &Conn{Conn: conn, readEarly: reqcopy}
			}
			s.handler(conn, addr)
		}(conn)
	}
}

func (s *Server) trackConn(c net.Conn) {
	s.mu.Lock()
	s.activeConn[c] = struct{}{}
	s.mu.Unlock()
}

func (s *Server) untrackConn(c net.Conn) {
	s.mu.Lock()
	delete(s.activeConn, c)
	s.mu.Unlock()
}

func (s *Server) Close() error {
	close(s.done)
	var err error
	if s.lis != nil {
		err = s.lis.Close()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for c := range s.activeConn {
		c.Close()
		delete(s.activeConn, c)
	}
	return err
}

// HTTP代理服务器接收到请求时：
// - 当方法是 CONNECT 时，即是HTTPS代理请求，服务端只需回应连接建立成功，后续原封不动地转发客户端数据即可
// - 其他方法则是 HTTP 代理请求，服务端需要先把请求内容转发到远端服务器，后续原封不动地转发客户端数据即可
func (s *Server) handshake(conn net.Conn) (addr string, reqcopy []byte, err error) {
	if err = conn.SetDeadline(time.Now().Add(common.HandshakeTimeout)); err != nil {
		return
	}
	defer func() {
		if e := conn.SetDeadline(time.Time{}); e != nil && err == nil {
			err = e
		}
	}()

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
		if _, err = fmt.Fprintf(conn, "%s 200 Connection established\r\n\r\n", req.Proto); err != nil {
			return "", nil, err
		}
	} else {
		if port == "" {
			port = "80"
		}
		// forward http proxy request
		var buf bytes.Buffer
		if err = req.Write(&buf); err != nil {
			return "", nil, err
		}
		reqcopy = buf.Bytes()
	}

	addr = net.JoinHostPort(req.URL.Hostname(), port)
	return addr, reqcopy, nil
}

// Conn wraps net.Conn, implements early-read especially for HTTP proxy forwarding
type Conn struct {
	net.Conn
	readEarly []byte // data to be read early before reading the underlying connection
	off       int
}

func (c *Conn) Read(b []byte) (n int, err error) {
	if c.off < len(c.readEarly) {
		n := copy(b, c.readEarly[c.off:])
		c.off += n
		return n, nil
	}

	return c.Conn.Read(b)
}

type writerOnly struct {
	io.Writer
}

func (c *Conn) ReadFrom(r io.Reader) (n int64, err error) {
	if rf, ok := c.Conn.(io.ReaderFrom); ok {
		return rf.ReadFrom(r)
	}
	// Use wrapper to hide existing c.ReadFrom from io.Copy.
	return io.Copy(writerOnly{c}, r)
}

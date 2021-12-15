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

type Server struct {
	addr    string
	handler common.Handler
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

func (s *Server) Handle(handler common.Handler) {
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

		go func() {
			s.trackConn(conn, true)
			defer s.trackConn(conn, false)
			newConn, addr, err := s.handshake(conn)
			if err != nil {
				log.L().Errorf("handshake: %s", err.Error())
				conn.Close()
				return
			}
			s.handler(newConn, addr)
		}()
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
func (s *Server) handshake(conn net.Conn) (newConn net.Conn, addr string, err error) {
	err = conn.SetDeadline(time.Now().Add(common.HandshakeTimeout))
	if err != nil {
		return
	}
	defer func() {
		er := conn.SetDeadline(time.Time{})
		if er != nil && err == nil {
			err = er
		}
	}()

	req, err := http.ReadRequest(bufio.NewReader(conn))
	if err != nil {
		return
	}

	if req.Method == "CONNECT" {
		addr, err = s.handshakeHTTPS(conn, req)
		if err != nil {
			return
		}
		newConn = conn
	} else {
		var buf bytes.Buffer
		addr, buf, err = s.handshakeHTTP(conn, req)
		if err != nil {
			return
		}
		newConn = &Conn{Conn: conn, earlyRead: buf}
	}

	return newConn, addr, nil
}

func (s *Server) handshakeHTTPS(conn net.Conn, req *http.Request) (addr string, err error) {
	port := req.URL.Port()
	if port == "" {
		port = "443"
	}
	addr = net.JoinHostPort(req.URL.Hostname(), port)

	_, err = fmt.Fprintf(conn, "%s 200 Connection established\r\n\r\n", req.Proto)
	if err != nil {
		return "", err
	}

	return addr, nil
}

func (s *Server) handshakeHTTP(conn net.Conn, req *http.Request) (addr string, buf bytes.Buffer, err error) {
	port := req.URL.Port()
	if port == "" {
		port = "80"
	}
	addr = net.JoinHostPort(req.URL.Hostname(), port)

	// 对于HTTP代理请求，需要先行把请求转发一遍
	if err = req.Write(&buf); err != nil {
		err = errors.New("request write err: " + err.Error())
		return
	}

	return addr, buf, nil
}

type Conn struct {
	net.Conn
	earlyRead bytes.Buffer // the buffer to be read early before Read
}

func (c *Conn) Read(b []byte) (n int, err error) {
	if c.earlyRead.Len() > 0 {
		return c.earlyRead.Read(b)
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

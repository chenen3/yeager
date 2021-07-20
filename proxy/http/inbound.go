package http

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	glog "log"
	"net"
	"net/http"
	"strconv"
	"time"

	"yeager/log"
	"yeager/proxy"
)

type Server struct {
	conf   *Config
	connCh chan proxy.Conn
	done   chan struct{}
	ready  chan struct{}
}

func NewServer(conf *Config) *Server {
	return &Server{
		conf:   conf,
		connCh: make(chan proxy.Conn, 32),
		done:   make(chan struct{}),
		ready:  make(chan struct{}),
	}
}

func (s *Server) Accept() <-chan proxy.Conn {
	return s.connCh
}

func (s *Server) Close() error {
	close(s.done)
	close(s.connCh)
	for conn := range s.connCh {
		conn.Close()
	}
	return nil
}

func (s *Server) Serve() {
	addr := net.JoinHostPort(s.conf.Host, strconv.Itoa(s.conf.Port))
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Errorf("http proxy failed to listen, err: %s", err)
		return
	}
	defer ln.Close()
	glog.Println("http proxy listening on", addr)

	close(s.ready)
	for {
		select {
		case <-s.done:
			return
		default:
		}

		conn, err := ln.Accept()
		if err != nil {
			log.Error(err)
			continue
		}
		go s.handleConnection(conn)
	}
}

func (s *Server) handleConnection(conn net.Conn) {
	newConn, err := s.handshake(conn)
	if err != nil {
		log.Error("failed to handshake: " + err.Error())
		conn.Close()
		return
	}

	select {
	case <-s.done:
		newConn.Close()
		return
	case s.connCh <- newConn:
	}
}

// HTTP代理服务器接收到请求时：
// - 当方法是 CONNECT 时，即是HTTPS代理请求，服务端只需回应连接建立成功，后续原封不动地转发客户端数据即可
// - 其他方法则是 HTTP 代理请求，服务端需要先把请求内容转发到远端服务器，后续原封不动地转发客户端数据即可
func (s *Server) handshake(conn net.Conn) (newConn proxy.Conn, err error) {
	err = conn.SetDeadline(time.Now().Add(proxy.HandshakeTimeout))
	if err != nil {
		return nil, err
	}
	defer func() {
		er := conn.SetDeadline(time.Time{})
		if er != nil && err == nil {
			err = er
		}
	}()

	req, err := http.ReadRequest(bufio.NewReader(conn))
	if err != nil {
		return nil, err
	}

	if req.Method == "CONNECT" {
		dst, err := s.handshakeHTTPS(conn, req)
		if err != nil {
			return nil, err
		}
		newConn = &Conn{Conn: conn, dst: dst}
	} else {
		dst, buf, err := s.handshakeHTTP(conn, req)
		if err != nil {
			return nil, err
		}
		newConn = &Conn{Conn: conn, dst: dst, earlyRead: buf}
	}

	return newConn, nil
}

func (s *Server) handshakeHTTPS(conn net.Conn, req *http.Request) (*proxy.Address, error) {
	host := req.URL.Hostname()
	port := req.URL.Port()
	if port == "" {
		port = "443"
	}
	portNum, err := strconv.Atoi(port)
	if err != nil {
		return nil, errors.New("invalid port: " + port)
	}
	dstAddr := proxy.NewAddress(host, portNum)

	_, err = fmt.Fprintf(conn, "%s 200 Connection established\r\n\r\n", req.Proto)
	if err != nil {
		return nil, err
	}
	return dstAddr, nil
}

func (s *Server) handshakeHTTP(conn net.Conn, req *http.Request) (dst *proxy.Address, buf bytes.Buffer, err error) {
	host := req.URL.Hostname()
	port := req.URL.Port()
	if port == "" {
		port = "80"
	}
	portNum, err := strconv.Atoi(port)
	if err != nil {
		err = errors.New("invalid port: " + port)
		return
	}
	dst = proxy.NewAddress(host, portNum)

	// 对于HTTP代理请求，需要先行把请求转发一遍
	if err = req.Write(&buf); err != nil {
		err = errors.New("request write err: " + err.Error())
		return
	}

	return dst, buf, nil
}

type Conn struct {
	net.Conn
	dst       *proxy.Address
	earlyRead bytes.Buffer // the buffer to be read early before Read
}

func (c *Conn) DstAddr() *proxy.Address {
	return c.dst
}

func (c *Conn) Read(b []byte) (n int, err error) {
	if c.earlyRead.Len() > 0 {
		return c.earlyRead.Read(b)
	}

	return c.Conn.Read(b)
}

func (c *Conn) ReadFrom(r io.Reader) (n int64, err error) {
	if rf, ok := c.Conn.(io.ReaderFrom); ok {
		return rf.ReadFrom(r)
	}
	return io.Copy(c.Conn, r)
}

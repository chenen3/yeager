package http

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	glog "log"
	"net"
	"net/http"
	"strconv"

	"yeager/log"
	"yeager/protocol"
)

type Server struct {
	conf   *Config
	connCh chan protocol.Conn
	done   chan struct{}
}

func NewServer(conf *Config) *Server {
	return &Server{
		conf:   conf,
		connCh: make(chan protocol.Conn, 32),
		done:   make(chan struct{}),
	}
}

func (s *Server) Accept() <-chan protocol.Conn {
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
		log.Errorf("http proxy failed to listen on %s, err: %s", addr, err)
		return
	}
	defer ln.Close()
	glog.Println("http proxy listening on", addr)

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
		log.Error(err)
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
func (s *Server) handshake(conn net.Conn) (protocol.Conn, error) {
	req, err := http.ReadRequest(bufio.NewReader(conn))
	if err != nil {
		return nil, err
	}

	if req.Method == "CONNECT" {
		return s.handshakeHTTPS(conn, req)
	} else {
		return s.handshakeHTTP(conn, req)
	}
}

func (s *Server) handshakeHTTPS(conn net.Conn, req *http.Request) (protocol.Conn, error) {
	host := req.URL.Hostname()
	port := req.URL.Port()
	if port == "" {
		port = "443"
	}
	portnum, err := strconv.Atoi(port)
	if err != nil {
		return nil, errors.New("invalid port: " + port)
	}
	dstAddr := protocol.NewAddress(host, portnum)

	_, err = fmt.Fprintf(conn, "%s 200 Connection established\r\n\r\n", req.Proto)
	if err != nil {
		return nil, err
	}
	newConn := protocol.NewConn(conn, dstAddr)
	return newConn, nil
}

func (s *Server) handshakeHTTP(conn net.Conn, req *http.Request) (protocol.Conn, error) {
	host := req.URL.Hostname()
	port := req.URL.Port()
	if port == "" {
		port = "80"
	}
	portnum, err := strconv.Atoi(port)
	if err != nil {
		return nil, errors.New("invalid port: " + port)
	}
	dstAddr := protocol.NewAddress(host, portnum)

	pconn := newPipeConn(conn, dstAddr)
	go func(pconn *pipeConn) {
		// 对于HTTP代理请求，需要先行把请求数据转发一遍
		if err := req.Write(pconn.pw); err != nil {
			log.Error(err)
		}
		pconn.pw.Close()
	}(pconn)
	return pconn, nil
}

// pipeConn implement Reader which read it's pipeReader firstly, then the underlying net.Conn
type pipeConn struct {
	net.Conn
	dstAddr *protocol.Address
	pr      *io.PipeReader
	pw      *io.PipeWriter
}

func newPipeConn(conn net.Conn, addr *protocol.Address) *pipeConn {
	pipeReader, pipeWriter := io.Pipe()
	return &pipeConn{
		Conn:    conn,
		dstAddr: addr,
		pr:      pipeReader,
		pw:      pipeWriter,
	}
}

func (c *pipeConn) DstAddr() *protocol.Address {
	return c.dstAddr
}

func (c *pipeConn) Read(p []byte) (n int, err error) {
	n, err = c.pr.Read(p)
	if err != nil && err != io.EOF {
		return n, err
	}
	if err == io.EOF {
		if n < len(p) {
			m, merr := c.Conn.Read(p[n:])
			return n + m, merr
		}
		return n, nil
	}
	return n, nil
}

func (c *pipeConn) Close() error {
	c.pw.Close()
	c.pr.Close()
	return c.Conn.Close()
}
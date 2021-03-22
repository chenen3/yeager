package http

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"

	"yeager/log"
	"yeager/protocol"
)

type Server struct {
	conf   *Config
	connCh chan protocol.Conn
	ctx    context.Context
	cancel context.CancelFunc
}

// TODO: 是否接收context参数？
func NewServer(conf *Config) *Server {
	s := &Server{
		conf:   conf,
		connCh: make(chan protocol.Conn, 32),
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.ctx = ctx
	s.cancel = cancel

	go func() {
		log.Error(s.listenAndServe())
		s.cancel()
	}()
	return s
}

func (s *Server) Accept() <-chan protocol.Conn {
	return s.connCh
}

func (s *Server) Close() error {
	s.cancel()
	close(s.connCh)
	return nil
}

func (s *Server) listenAndServe() error {
	ln, err := net.Listen("tcp", net.JoinHostPort(s.conf.Host, strconv.Itoa(s.conf.Port)))
	if err != nil {
		return err
	}
	defer ln.Close()

	for {
		select {
		case <-s.ctx.Done():
			return nil
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
		newConn.Close()
		return
	}

	// in case send on closed channel
	select {
	case <-s.ctx.Done():
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

	var addr string
	if req.Method == "CONNECT" { // https
		addr = req.Host // 这里host包含端口
		_, err = fmt.Fprintf(conn, "HTTP/%d.%d 200 Connection established\r\n\r\n", req.ProtoMajor, req.ProtoMinor)
		if err != nil {
			return nil, err
		}
		// TODO 解析域名的工作留给"按规则分流"做
		a, _ := net.ResolveTCPAddr("tcp", addr)
		newConn := protocol.NewConn(conn, a)
		return newConn, nil
	} else { // http
		addr = net.JoinHostPort(req.Host, "80")
		pipeReader, pipeWriter := io.Pipe()
		newConn := &Conn{
			Conn:       conn,
			dstAddr:    addr,
			pipeReader: pipeReader,
			pipeWriter: pipeWriter,
		}
		go func(newConn *Conn) {
			if err := req.Write(newConn.pipeWriter); err != nil {
				log.Error(err)
			}
			newConn.pipeWriter.Close()
		}(newConn)
		return newConn, nil
	}
}

type Conn struct {
	net.Conn
	dstAddr    string
	pipeReader *io.PipeReader
	pipeWriter *io.PipeWriter
	pipeDone   bool
}

func (c *Conn) DstAddr() net.Addr {
	// TODO: do not resolve domain
	a, _ := net.ResolveTCPAddr("tcp", c.dstAddr)
	return a
}

func (c *Conn) Read(p []byte) (n int, err error) {
	if c.pipeDone {
		return c.Conn.Read(p)
	}

	n, err = c.pipeReader.Read(p)
	switch err {
	case nil:
		return n, nil
	case io.EOF:
		c.pipeDone = true
		if n < len(p) {
			m, merr := c.Conn.Read(p[n:])
			return n + m, merr
		}
		return n, nil
	default:
		return n, err
	}
}

func (c *Conn) Close() error {
	c.pipeWriter.Close()
	c.pipeReader.Close()
	return c.Conn.Close()
}

package http

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	glog "log"
	"net"
	"net/http"
	"strconv"
	"time"

	"yeager/log"
	"yeager/proxy"
	"yeager/util"
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
	err := conn.SetDeadline(time.Now().Add(proxy.HandshakeTimeout))
	if err != nil {
		log.Error("failed to set handshake timeout: " + err.Error())
		conn.Close()
		return
	}

	newConn, err := s.handshake(conn)
	if err != nil {
		log.Error("failed to handshake: " + err.Error())
		newConn.Close()
		return
	}

	err = newConn.SetDeadline(time.Time{})
	if err != nil {
		log.Error("failed to clear handshake timeout: " + err.Error())
		newConn.Close()
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
func (s *Server) handshake(conn net.Conn) (proxy.Conn, error) {
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

func (s *Server) handshakeHTTPS(conn net.Conn, req *http.Request) (proxy.Conn, error) {
	host := req.URL.Hostname()
	port := req.URL.Port()
	if port == "" {
		port = "443"
	}
	portnum, err := strconv.Atoi(port)
	if err != nil {
		return nil, errors.New("invalid port: " + port)
	}
	dstAddr := proxy.NewAddress(host, portnum)

	_, err = fmt.Fprintf(conn, "%s 200 Connection established\r\n\r\n", req.Proto)
	if err != nil {
		return nil, err
	}
	newConn := proxy.NewConn(conn, dstAddr)
	return newConn, nil
}

func (s *Server) handshakeHTTP(conn net.Conn, req *http.Request) (proxy.Conn, error) {
	host := req.URL.Hostname()
	port := req.URL.Port()
	if port == "" {
		port = "80"
	}
	portNum, err := strconv.Atoi(port)
	if err != nil {
		return nil, errors.New("invalid port: " + port)
	}
	dstAddr := proxy.NewAddress(host, portNum)

	buf := new(bytes.Buffer)
	// 对于HTTP代理请求，需要先行把请求转发一遍
	if err := req.Write(buf); err != nil {
		return nil, errors.New("request write err: " + err.Error())
	}
	erc := util.EarlyReadConn(conn, buf)
	return proxy.NewConn(erc, dstAddr), nil
}

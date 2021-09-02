package http

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"yeager/config"
	"yeager/log"
	"yeager/proxy"
)

type Server struct {
	ctx    context.Context
	cancel context.CancelFunc
	conf   *config.HTTPServerConfig
	wg     sync.WaitGroup // counts active Serve goroutines for graceful close
	lis    net.Listener

	ready chan struct{} // imply that server is ready to accept connection, testing only
}

func NewServer(conf *config.HTTPServerConfig) *Server {
	ctx, cancel := context.WithCancel(context.Background())
	return &Server{
		conf:   conf,
		ready:  make(chan struct{}),
		ctx:    ctx,
		cancel: cancel,
	}
}

func (s *Server) ListenAndServe(handle proxy.Handler) error {
	lis, err := net.Listen("tcp", s.conf.Address)
	if err != nil {
		return fmt.Errorf("http proxy failed to listen, err: %s", err)
	}
	s.lis = lis
	log.Infof("http proxy listening on %s", s.conf.Address)

	close(s.ready)
	for {
		select {
		case <-s.ctx.Done():
			return nil
		default:
		}

		conn, err := lis.Accept()
		if err != nil {
			log.Warn(err)
			continue
		}

		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			newConn, dst, err := s.handshake(conn)
			if err != nil {
				log.Error("handshake: " + err.Error())
				conn.Close()
				return
			}

			handle(s.ctx, newConn, dst)
		}()
	}
}

func (s *Server) Close() error {
	defer s.wg.Wait()
	s.cancel()
	return s.lis.Close()
}

// HTTP代理服务器接收到请求时：
// - 当方法是 CONNECT 时，即是HTTPS代理请求，服务端只需回应连接建立成功，后续原封不动地转发客户端数据即可
// - 其他方法则是 HTTP 代理请求，服务端需要先把请求内容转发到远端服务器，后续原封不动地转发客户端数据即可
func (s *Server) handshake(conn net.Conn) (newConn net.Conn, dst *proxy.Address, err error) {
	err = conn.SetDeadline(time.Now().Add(proxy.HandshakeTimeout))
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
		dst, err = s.handshakeHTTPS(conn, req)
		if err != nil {
			return
		}
		newConn = conn
	} else {
		var buf bytes.Buffer
		dst, buf, err = s.handshakeHTTP(conn, req)
		if err != nil {
			return
		}
		newConn = &Conn{Conn: conn, earlyRead: buf}
	}

	return newConn, dst, nil
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

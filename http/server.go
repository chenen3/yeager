package http

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/chenen3/yeager/util"
)

// ProxyServer implements the Inbounder interface
type ProxyServer struct {
	addr    string
	handler func(ctx context.Context, c net.Conn, addr string)
	lis     net.Listener

	wg     sync.WaitGroup
	ctx    context.Context
	cancel context.CancelFunc
	ready  chan struct{} // imply that server is ready to accept connection, testing only
}

func NewProxyServer(addr string) (*ProxyServer, error) {
	if addr == "" {
		return nil, errors.New("empty address")
	}

	ctx, cancel := context.WithCancel(context.Background())
	return &ProxyServer{
		addr:   addr,
		ready:  make(chan struct{}),
		ctx:    ctx,
		cancel: cancel,
	}, nil
}

func (s *ProxyServer) Handle(handler func(ctx context.Context, c net.Conn, addr string)) {
	s.handler = handler
}

func (s *ProxyServer) ListenAndServe() error {
	lis, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("http proxy listen: %s", err)
	}
	s.lis = lis
	log.Printf("http proxy listening %s", s.addr)

	close(s.ready)
	for {
		conn, err := lis.Accept()
		if err != nil {
			select {
			case <-s.ctx.Done():
				return nil
			default:
			}
			log.Printf("failed to accept conn: %s", err)
			continue
		}

		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			defer conn.Close()
			addr, reqcopy, err := s.handshake(conn)
			if err != nil {
				log.Printf("handshake: %s", err.Error())
				conn.Close()
				return
			}
			// forward HTTP proxy request
			if len(reqcopy) > 0 {
				conn = connWithLazyRead(conn, reqcopy)
			}
			s.handler(s.ctx, conn, addr)
		}()
	}
}

func (s *ProxyServer) Close() error {
	s.cancel()
	var err error
	if s.lis != nil {
		err = s.lis.Close()
	}
	return err
}

// Shutdown gracefully shuts down the server,
// it works by first closing listener,
// then wait for all connection to close
func (s *ProxyServer) Shutdown() error {
	s.cancel()
	var err error
	if s.lis != nil {
		err = s.lis.Close()
	}
	s.wg.Wait()
	return err
}

// HTTP代理服务器接收到请求时：
// - 当方法是 CONNECT 时，即是HTTPS代理请求，服务端只需回应连接建立成功，后续原封不动地转发客户端数据即可
// - 其他方法则是 HTTP 代理请求，服务端需要先把请求内容转发到远端服务器，后续原封不动地转发客户端数据即可
func (s *ProxyServer) handshake(conn net.Conn) (addr string, reqcopy []byte, err error) {
	if err = conn.SetDeadline(time.Now().Add(util.HandshakeTimeout)); err != nil {
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

// lazyReadConn wraps net.Conn, implements lazy-read for HTTP proxy forwarding
type lazyReadConn struct {
	net.Conn
	lazyRead []byte
}

// wrap connection with lazy-read data, which will not be read until the first Read()
func connWithLazyRead(conn net.Conn, lazyRead []byte) *lazyReadConn {
	return &lazyReadConn{
		Conn:     conn,
		lazyRead: lazyRead,
	}
}

func (c *lazyReadConn) Read(b []byte) (n int, err error) {
	if len(c.lazyRead) != 0 {
		n := copy(b, c.lazyRead)
		c.lazyRead = c.lazyRead[n:]
		return n, nil
	}

	return c.Conn.Read(b)
}

type writerOnly struct {
	io.Writer
}

func (c *lazyReadConn) ReadFrom(r io.Reader) (n int64, err error) {
	if rf, ok := c.Conn.(io.ReaderFrom); ok {
		return rf.ReadFrom(r)
	}
	// Use wrapper to hide existing c.ReadFrom from io.Copy.
	return io.Copy(writerOnly{c}, r)
}

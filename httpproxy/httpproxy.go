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
	"sync"
	"time"

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
func (s *Server) Serve(address string, d tunnel.Dialer) error {
	lis, err := net.Listen("tcp", address)
	if err != nil {
		return err
	}
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
	dstAddr, httpReq, err := handshake(conn)
	conn.SetDeadline(time.Time{})
	if err != nil {
		log.Printf("handshake: %s", err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), ynet.DialTimeout)
	defer cancel()
	rwc, err := d.DialContext(ctx, dstAddr)
	if err != nil {
		log.Printf("dial %s: %s", dstAddr, err)
		return
	}
	defer rwc.Close()

	if httpReq != nil {
		if err := httpReq.Write(rwc); err != nil {
			log.Print(err)
			return
		}
	}
	ch := make(chan error, 2)
	r := ynet.NewRelayer(conn, rwc)
	go r.ToDst(ch)
	go r.FromDst(ch)
	if err := <-ch; err != nil {
		log.Printf("relay %s: %s", dstAddr, err)
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

// HTTP代理服务器接收到请求时：
// - 当方法是 CONNECT 时，即是HTTPS代理请求，服务端只需回应连接建立成功，后续原封不动地转发客户端数据即可
// - 其他方法则是 HTTP 代理请求，服务端需要先把请求内容转发到远端服务器，后续原封不动地转发客户端数据即可
func handshake(conn net.Conn) (addr string, httpReq *http.Request, err error) {
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

	addr = net.JoinHostPort(req.URL.Hostname(), port)
	return addr, httpReq, nil
}

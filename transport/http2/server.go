package http2

import (
	"context"
	"crypto/subtle"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/chenen3/yeager/flow"
	"github.com/chenen3/yeager/logger"
	"golang.org/x/net/http2"
)

type Server struct {
	mu  sync.Mutex
	lis net.Listener
}

// Serve blocks until closed, or error occurs.
func (s *Server) Serve(address string, cfg *tls.Config, username, password string) error {
	cfg.NextProtos = []string{http2.NextProtoTLS}
	lis, err := net.Listen("tcp", address)
	if err != nil {
		return err
	}

	s.mu.Lock()
	s.lis = lis
	s.mu.Unlock()
	h2s := &http2.Server{IdleTimeout: 10 * time.Minute}
	var h handler
	if username != "" {
		h.auth = []byte(makeBasicAuth(username, password))
	}
	for {
		conn, err := lis.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				err = nil
			}
			return err
		}

		tlsConn := tls.Server(conn, cfg)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err = tlsConn.HandshakeContext(ctx)
		cancel()
		if err != nil {
			logger.Error.Printf("tls handshake: %s", err)
			tlsConn.Close()
			continue
		}

		go h2s.ServeConn(tlsConn, &http2.ServeConnOpts{
			Handler: h,
		})
	}
}

func (s *Server) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lis != nil {
		return s.lis.Close()
	}
	return nil
}

type handler struct {
	auth []byte
}

// works like HTTPS proxy server
func (h handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodConnect {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if r.Host == "" {
		http.Error(w, "missing host", http.StatusBadRequest)
		return
	}
	if len(h.auth) != 0 {
		auth := r.Header.Get("Proxy-Authorization")
		const prefix = "Basic "
		if len(auth) <= len(prefix) || !strings.EqualFold(auth[:len(prefix)], prefix) ||
			subtle.ConstantTimeCompare([]byte(auth[len(prefix):]), h.auth[len(prefix):]) != 1 {
			w.WriteHeader(http.StatusProxyAuthRequired)
			return
		}
	}

	w.WriteHeader(http.StatusOK)
	if f, ok := w.(http.Flusher); ok {
		// client is waiting for this header
		f.Flush()
	}

	targetConn, err := net.DialTimeout("tcp", r.Host, 5*time.Second)
	if err != nil {
		logger.Error.Print(err)
		return
	}
	defer targetConn.Close()
	go func() {
		flow.Copy(targetConn, r.Body)
		tcpConn, _ := targetConn.(*net.TCPConn)
		tcpConn.CloseWrite()
	}()
	flow.Copy(&flushWriter{w}, targetConn)
}

type flushWriter struct {
	http.ResponseWriter
}

func (w *flushWriter) Write(b []byte) (int, error) {
	n, err := w.ResponseWriter.Write(b)
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
	return n, err
}

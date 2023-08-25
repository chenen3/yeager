package http2

import (
	"context"
	"crypto/subtle"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/chenen3/yeager/forward"
	"golang.org/x/net/http2"
)

type TunnelServer struct {
	mu  sync.Mutex
	lis net.Listener
}

// Serve blocks until closed, or error occurs.
func (s *TunnelServer) Serve(address string, cfg *tls.Config, username, password string) error {
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
	if username != "" && password != "" {
		auth := username + ":" + password
		buf := make([]byte, base64.StdEncoding.EncodedLen(len(auth)))
		base64.StdEncoding.Encode(buf, []byte(auth))
		h.auth = buf
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
			slog.Error("tls handshake: " + err.Error())
			tlsConn.Close()
			continue
		}

		go h2s.ServeConn(tlsConn, &http2.ServeConnOpts{
			Handler: h,
		})
	}
}

type handler struct {
	auth []byte
}

// works like HTTPS proxy server
func (h handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Host == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if len(h.auth) != 0 {
		pa := r.Header.Get("Proxy-Authorization")
		const prefix = "Basic "
		if len(pa) <= len(prefix) || !strings.EqualFold(pa[:len(prefix)], prefix) ||
			subtle.ConstantTimeCompare([]byte(pa[len(prefix):]), h.auth) != 1 {
			// do not rely 407, which implys a proxy server
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	}

	w.WriteHeader(http.StatusOK)
	if f, ok := w.(http.Flusher); ok {
		// client is waiting for this response header,
		// send it as soon as possible
		f.Flush()
	}

	conn, err := net.DialTimeout("tcp", r.Host, 5*time.Second)
	if err != nil {
		slog.Error(err.Error())
		return
	}
	defer conn.Close()
	go func() {
		forward.Copy(conn, r.Body)
		conn.Close()
	}()
	// after Handler finished, calling http2.responseWriter.Flush() may panic
	forward.Copy(&flushWriter{w}, conn)
}

func (s *TunnelServer) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lis != nil {
		return s.lis.Close()
	}
	return nil
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

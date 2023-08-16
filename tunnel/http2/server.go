package http2

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	ynet "github.com/chenen3/yeager/net"
	"golang.org/x/net/http2"
)

type TunnelServer struct {
	mu  sync.Mutex
	lis net.Listener
}

// Serve blocks until closed, or error occurs.
func (s *TunnelServer) Serve(address string, tlsConf *tls.Config, username, password string) error {
	tlsConf.NextProtos = []string{http2.NextProtoTLS}
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
		h.auth = basicAuth(username, password)
	}
	for {
		conn, err := lis.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				err = nil
			}
			return err
		}

		tlsConn := tls.Server(conn, tlsConf)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err = tlsConn.HandshakeContext(ctx)
		cancel()
		if err != nil {
			log.Printf("tls handshake: %s", err)
			tlsConn.Close()
			continue
		}

		go h2s.ServeConn(tlsConn, &http2.ServeConnOpts{
			Handler: h,
		})
	}
}

type handler struct {
	auth string
}

// works like HTTPS proxy server
func (h handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Host == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if h.auth != "" {
		pa := r.Header.Get("Proxy-Authorization")
		if pa == "" {
			// do not rely 407, which implys a proxy server
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if s := strings.Split(pa, " "); len(s)%2 != 0 || s[0] != "Basic" || s[1] != h.auth {
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

	remote, err := net.Dial("tcp", r.Host)
	if err != nil {
		log.Print(err)
		return
	}
	defer remote.Close()
	go func() {
		ynet.Copy(remote, r.Body)
		remote.Close()
	}()
	// do not write response body in other goroutine, because calling
	// http2.responseWriter.Flush() after Handler finished may panic
	ynet.Copy(&flushWriter{w}, remote)
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

type readwriter struct {
	r io.Reader
	w io.Writer
}

func (rw *readwriter) Read(p []byte) (int, error) {
	return rw.r.Read(p)
}

func (rw *readwriter) Write(p []byte) (int, error) {
	return rw.w.Write(p)
}

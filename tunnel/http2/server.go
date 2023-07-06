package http2

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
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
func (s *TunnelServer) Serve(address string, tlsConf *tls.Config) error {
	tlsConf.NextProtos = []string{http2.NextProtoTLS}
	lis, err := net.Listen("tcp", address)
	if err != nil {
		return err
	}

	s.mu.Lock()
	s.lis = lis
	s.mu.Unlock()
	h2s := &http2.Server{IdleTimeout: 5 * time.Minute}
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
			Handler: http.HandlerFunc(serveHTTP),
		})
	}
}

func serveHTTP(w http.ResponseWriter, r *http.Request) {
	dst := r.Header.Get("dst")
	remote, err := net.Dial("tcp", dst)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		log.Print(err)
		return
	}
	defer remote.Close()

	w.WriteHeader(http.StatusOK)
	if f, ok := w.(http.Flusher); ok {
		// the client waits for the response header
		f.Flush()
	}

	local := &readwriter{r.Body, &flushWriter{w}}
	if err = ynet.Relay(local, remote); err != nil {
		if errors.Is(err, net.ErrClosed) {
			return
		}
		if se, ok := err.(http2.StreamError); ok && se.Code == http2.ErrCodeCancel {
			return
		}
		log.Printf("relay %s: %s", dst, err)
	}
}

func (s *TunnelServer) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lis.Close()
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

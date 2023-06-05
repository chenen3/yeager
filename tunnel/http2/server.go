package http2

import (
	"context"
	"crypto/tls"
	"errors"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/chenen3/yeager/debug"
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
	remote, err := net.Dial("tcp", r.Header.Get("dst"))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		log.Print(err)
		return
	}

	w.WriteHeader(http.StatusOK)
	if f, ok := w.(http.Flusher); ok {
		// If this is not done,
		// the client will wait for the request to complete
		// and cannot delay writing to the request body.
		f.Flush()
	}

	done := make(chan struct{})
	go func() {
		_, e := ynet.Copy(remote, r.Body)
		if e != nil {
			se, ok := e.(http2.StreamError)
			if !ok || se.Code != http2.ErrCodeCancel {
				debug.Printf("send: %s", e)
			}
		}
		// unblock Read on remote
		remote.Close()
		close(done)
	}()

	_, err = ynet.Copy(&flushWriter{w}, remote)
	if err != nil && !errors.Is(err, net.ErrClosed) {
		debug.Printf("receive: %s", err)
	}
	// unblock Read on r.Body
	r.Body.Close()
	<-done
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

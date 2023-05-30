package http2

import (
	"crypto/tls"
	"errors"
	"log"
	"net"
	"net/http"

	// "net/http/httputil"
	"sync"
	"time"

	ynet "github.com/chenen3/yeager/net"
	"golang.org/x/net/http2"
)

type TunnelServer struct {
	mu  sync.Mutex
	lis net.Listener
}

// Serve will return a non-nil error unless Close is called.
func (s *TunnelServer) Serve(address string, tlsConf *tls.Config) error {
	tlsConf.NextProtos = []string{"h2"}
	lis, err := net.Listen("tcp", address)
	if err != nil {
		return err
	}

	s.mu.Lock()
	s.lis = lis
	s.mu.Unlock()
	h2srv := &http2.Server{IdleTimeout: 5 * time.Minute}
	for {
		conn, err := lis.Accept()
		if err != nil {
			return err
		}
		tlsConn := tls.Server(conn, tlsConf)
		if err := tlsConn.Handshake(); err != nil {
			log.Print(err)
			continue
		}

		h2srv.ServeConn(tlsConn, &http2.ServeConnOpts{
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
	if flusher, ok := w.(http.Flusher); ok {
		// flush the buffered headers to the client,
		// otherwise the client may not start to read
		flusher.Flush()
	}

	done := make(chan struct{})
	go func() {
		_, e := ynet.Copy(remote, r.Body)
		if e != nil && !errors.Is(e, net.ErrClosed) {
			log.Printf("copy to remote: %s", e)
		}
		// unblock Read on remote
		remote.Close()
		close(done)
	}()

	_, err = ynet.Copy(&flushWriter{w}, remote)
	if err != nil && !errors.Is(err, net.ErrClosed) {
		log.Printf("copy from remote: %s", err)
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
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
	return n, err
}

package wss

import (
	"crypto/tls"
	"errors"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	ynet "github.com/chenen3/yeager/net"
	"github.com/chenen3/yeager/tunnel"
	"golang.org/x/net/websocket"
)

type TunnelServer struct {
	hs *http.Server
	mu sync.Mutex
}

func (s *TunnelServer) Serve(l net.Listener, tlsConf *tls.Config) error {
	mux := http.NewServeMux()
	mux.Handle("/relay", websocket.Handler(handleConn))
	hs := &http.Server{
		TLSConfig: tlsConf,
		Handler:   mux,
	}
	s.mu.Lock()
	s.hs = hs
	s.mu.Unlock()
	err := hs.ServeTLS(l, "", "")
	if errors.Is(err, http.ErrServerClosed) {
		err = nil
	}
	return err
}

func handleConn(ws *websocket.Conn) {
	defer ws.Close()
	ws.SetReadDeadline(time.Now().Add(ynet.HandshakeTimeout))
	dst, err := tunnel.ReadHeader(ws)
	if err != nil {
		log.Printf("read header: %s", err)
		return
	}
	ws.SetReadDeadline(time.Time{})

	remote, err := net.DialTimeout("tcp", dst, ynet.DialTimeout)
	if err != nil {
		log.Print(err)
		return
	}
	defer remote.Close()

	if _, _, err := ynet.Relay(ws, remote); err != nil {
		log.Printf("relay %s: %s", dst, err)
	}
}

func (s *TunnelServer) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.hs == nil {
		return nil
	}
	return s.hs.Close()
}

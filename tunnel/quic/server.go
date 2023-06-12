package quic

import (
	"context"
	"crypto/tls"
	"errors"
	"log"
	"net"
	"sync"
	"time"

	ynet "github.com/chenen3/yeager/net"
	"github.com/chenen3/yeager/tunnel"
	"github.com/quic-go/quic-go"
)

// Idleness duration is defined since no incomming network activity,
// quic-go doesn't act like the grpc way that checking the number of streams.
// Increase this duration to 5 minutes so the connection doesn't get closed too soon
const idleTimeout = 5 * time.Minute

// TunnelServer is a QUIC tunnel server, its zero value is ready to use
type TunnelServer struct {
	mu  sync.Mutex
	lis quic.Listener
}

// Serve will return a non-nil error unless Close is called.
func (s *TunnelServer) Serve(address string, tlsConf *tls.Config) error {
	if tlsConf == nil {
		return errors.New("TLS config required")
	}
	tlsConf.NextProtos = append(tlsConf.NextProtos, "quic")
	conf := &quic.Config{MaxIdleTimeout: idleTimeout}
	lis, err := quic.ListenAddr(address, tlsConf, conf)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.lis = lis
	s.mu.Unlock()

	for {
		conn, err := lis.Accept(context.Background())
		if err != nil {
			if errors.Is(err, quic.ErrServerClosed) {
				err = nil
			}
			return err
		}
		go handleConn(conn)
	}
}

func handleConn(conn quic.Connection) {
	defer conn.CloseWithError(quic.ApplicationErrorCode(quic.NoError), "")
	for {
		stream, err := conn.AcceptStream(context.Background())
		if err != nil {
			if !errors.Is(err, net.ErrClosed) {
				log.Printf("accept quic stream: %s", err)
			}
			return
		}
		select {
		case <-stream.Context().Done():
			continue
		default:
		}
		go handleStream(stream)
	}
}

func handleStream(stream quic.Stream) {
	defer stream.Close()
	stream.SetReadDeadline(time.Now().Add(ynet.HandshakeTimeout))
	dst, err := tunnel.ReadHeader(stream)
	if err != nil {
		log.Printf("read header: %s", err)
		return
	}
	stream.SetReadDeadline(time.Time{})

	remote, err := net.DialTimeout("tcp", dst, ynet.DialTimeout)
	if err != nil {
		log.Print(err)
		return
	}
	defer remote.Close()

	err = ynet.Relay(stream, remote)
	if err != nil {
		if e, ok := err.(*quic.ApplicationError); ok && e.ErrorCode == 0 {
			return
		}
		log.Printf("relay %s: %s", dst, err)
	}
}

func (s *TunnelServer) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lis == nil {
		return nil
	}
	// closing the QUIC listener will close all active connections
	return s.lis.Close()
}

package quic

import (
	"context"
	"crypto/tls"
	"errors"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/chenen3/yeager/forward"
	"github.com/quic-go/quic-go"
)

// TunnelServer is a QUIC tunnel server, its zero value is ready to use
type TunnelServer struct {
	mu  sync.Mutex
	lis *quic.Listener
}

// Idleness duration is defined since no incomming network activity,
// quic-go doesn't act like the grpc way that checking the number of streams.
// Increase this duration to 5 minutes so the connection doesn't get closed too soon
const idleTimeout = 5 * time.Minute

var quicConfig = &quic.Config{
	MaxIdleTimeout: idleTimeout,
}

// Serve will return a non-nil error unless Close is called.
func (s *TunnelServer) Serve(address string, tlsConf *tls.Config) error {
	if tlsConf == nil {
		return errors.New("TLS config required")
	}
	tlsConf.NextProtos = append(tlsConf.NextProtos, "quic")
	lis, err := quic.ListenAddr(address, tlsConf, quicConfig)
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
				slog.Error("accept quic stream: " + err.Error())
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
	stream.SetReadDeadline(time.Now().Add(5 * time.Second))
	var m metadata
	if _, err := m.ReadFrom(stream); err != nil {
		slog.Error(err.Error())
		return
	}
	if m.Hostport == "" {
		slog.Error("empty target")
		return
	}
	target := m.Hostport
	stream.SetReadDeadline(time.Time{})

	conn, err := net.DialTimeout("tcp", target, 5*time.Second)
	if err != nil {
		slog.Error(err.Error())
		return
	}
	defer conn.Close()

	err = forward.Dual(stream, conn)
	if err != nil {
		if e, ok := err.(*quic.ApplicationError); ok && e.ErrorCode == 0 {
			return
		}
		slog.Error(err.Error(), "addr", target)
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

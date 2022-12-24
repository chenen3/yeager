package quic

import (
	"context"
	"crypto/tls"
	"errors"
	"log"
	"net"
	"sync"
	"time"

	ylog "github.com/chenen3/yeager/log"
	ynet "github.com/chenen3/yeager/net"
	"github.com/chenen3/yeager/tunnel"
	"github.com/lucas-clemente/quic-go"
)

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
	qConf := &quic.Config{MaxIdleTimeout: ynet.IdleConnTimeout}
	lis, err := quic.ListenAddr(address, tlsConf, qConf)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.lis = lis
	s.mu.Unlock()

	for {
		qconn, err := lis.Accept(context.Background())
		if err != nil {
			if errors.Is(err, quic.ErrServerClosed) {
				err = nil
			}
			return err
		}
		go func() {
			defer qconn.CloseWithError(quic.ApplicationErrorCode(quic.NoError), "")
			for {
				stream, err := qconn.AcceptStream(context.Background())
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
				go s.handleStream(stream)
			}
		}()
	}
}

func (s *TunnelServer) handleStream(stream quic.Stream) {
	defer stream.Close()
	stream.SetReadDeadline(time.Now().Add(ynet.HandshakeTimeout))
	dst, err := tunnel.ReadHeader(stream)
	stream.SetReadDeadline(time.Time{})
	if err != nil {
		log.Printf("read header: %s", err)
		return
	}

	remote, err := net.DialTimeout("tcp", dst, ynet.DialTimeout)
	if err != nil {
		log.Print(err)
		return
	}
	defer remote.Close()

	if _, _, err := ynet.Relay(stream, remote); err != nil {
		ylog.Debugf("relay %s: %s", dst, err)
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

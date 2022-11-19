package quic

import (
	"context"
	"crypto/tls"
	"errors"
	"log"
	"net"
	"sync"
	"time"

	"github.com/chenen3/yeager/relay"
	"github.com/chenen3/yeager/tunnel"
	"github.com/chenen3/yeager/util"
	"github.com/lucas-clemente/quic-go"
)

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
	qConf := &quic.Config{MaxIdleTimeout: util.MaxConnectionIdle}
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
	stream.SetReadDeadline(time.Now().Add(util.HandshakeTimeout))
	dstAddr, err := tunnel.ReadHeader(stream)
	stream.SetReadDeadline(time.Time{})
	if err != nil {
		log.Printf("read header: %s", err)
		return
	}

	dstConn, err := net.DialTimeout("tcp", dstAddr, util.DialTimeout)
	if err != nil {
		log.Print(err)
		return
	}
	defer dstConn.Close()

	ch := make(chan error, 2)
	r := relay.New(stream, dstConn)
	go r.ToDst(ch)
	go r.FromDst(ch)
	<-ch
}

func (s *TunnelServer) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lis != nil {
		// will close all active connections
		return s.lis.Close()
	}
	return nil
}

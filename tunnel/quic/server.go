package quic

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"log"
	"net"
	"time"

	"github.com/chenen3/yeager/tunnel"
	"github.com/chenen3/yeager/util"
	"github.com/lucas-clemente/quic-go"
)

type TunnelServer struct {
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
	s.lis = lis

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

	conn, err := net.DialTimeout("tcp", dstAddr, util.DialTimeout)
	if err != nil {
		log.Print(err)
		return
	}
	defer conn.Close()

	go io.Copy(conn, stream)
	io.Copy(stream, conn)
}

func (s *TunnelServer) Close() error {
	if s.lis != nil {
		return s.lis.Close()
	}
	return nil
}

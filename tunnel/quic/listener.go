package quic

import (
	"context"
	"crypto/tls"
	"errors"
	"log"
	"net"
	"sync"

	"github.com/lucas-clemente/quic-go"

	"github.com/chenen3/yeager/util"
)

// listener implements the net.Listener interface
type listener struct {
	ctx       context.Context
	cancelCtx context.CancelFunc
	lis       quic.Listener
	conns     chan net.Conn
	wg        sync.WaitGroup
}

func (l *listener) acceptLoop() {
	for {
		qconn, err := l.lis.Accept(l.ctx)
		if err != nil {
			if !errors.Is(err, context.Canceled) {
				log.Printf("accept quic connection: %s", err)
			}
			break
		}

		l.wg.Add(1)
		go func() {
			defer l.wg.Done()
			defer qconn.CloseWithError(quic.ApplicationErrorCode(quic.NoError), "")
			for {
				stream, err := qconn.AcceptStream(context.Background())
				if err != nil {
					if !errors.Is(err, net.ErrClosed) {
						log.Printf("accept quic stream: %s", err)
					}
					break
				}

				select {
				case <-stream.Context().Done():
					continue
				default:
				}
				l.conns <- &streamConn{
					Stream:     stream,
					localAddr:  qconn.LocalAddr(),
					remoteAddr: qconn.RemoteAddr(),
				}
			}
		}()
	}
}

func (l *listener) Accept() (net.Conn, error) {
	select {
	case <-l.ctx.Done():
		// do not return until all connection closed
		l.wg.Wait()
		return nil, net.ErrClosed
	case c := <-l.conns:
		return c, nil
	}
}

func (l *listener) Close() error {
	l.cancelCtx()
	return l.lis.Close()
}

func (l *listener) Addr() net.Addr {
	return l.lis.Addr()
}

// Listen creates a QUIC server listening on a given address
func Listen(addr string, tlsConf *tls.Config) (net.Listener, error) {
	if tlsConf == nil {
		return nil, errors.New("tls config required")
	}

	tlsConf.NextProtos = []string{"quic"}
	lis, err := quic.ListenAddr(addr, tlsConf, &quic.Config{MaxIdleTimeout: util.MaxConnectionIdle})
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	l := &listener{
		ctx:       ctx,
		cancelCtx: cancel,
		lis:       lis,
		conns:     make(chan net.Conn, 32),
	}
	go l.acceptLoop()
	return l, nil
}

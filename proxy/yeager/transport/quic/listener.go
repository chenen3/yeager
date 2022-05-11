package quic

import (
	"context"
	"crypto/tls"
	"errors"
	"log"
	"net"
	"sync"

	"github.com/lucas-clemente/quic-go"

	"github.com/chenen3/yeager/proxy/common"
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
			select {
			case <-l.ctx.Done():
				return
			default:
				log.Printf("failed to accept quic connection: %s", err)
				continue
			}
		}
		l.wg.Add(1)
		go l.acceptStream(qconn)
	}
}

func (l *listener) acceptStream(qconn quic.Connection) {
	defer func() {
		err := qconn.CloseWithError(quic.ApplicationErrorCode(quic.NoError), "connection closed")
		if err != nil {
			log.Printf("close quic connection: %s", err)
		}
		l.wg.Done()
	}()

	for {
		stream, err := qconn.AcceptStream(l.ctx)
		if err != nil {
			select {
			case <-l.ctx.Done():
				return
			case <-qconn.Context().Done():
				return
			default:
				log.Printf("failed to accept quic stream: %s", err)
				continue
			}
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
}

func (l *listener) Accept() (net.Conn, error) {
	select {
	case <-l.ctx.Done():
		// do not return until all connection closed
		l.wg.Wait()
		return nil, l.ctx.Err()
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

func Listen(addr string, tlsConf *tls.Config) (net.Listener, error) {
	if tlsConf == nil {
		return nil, errors.New("tls config required")
	}

	tlsConf.NextProtos = []string{"quic"}
	lis, err := quic.ListenAddr(addr, tlsConf, &quic.Config{MaxIdleTimeout: common.MaxConnectionIdle})
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

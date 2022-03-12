package quic

import (
	"context"
	"crypto/tls"
	"errors"
	"net"

	quic "github.com/lucas-clemente/quic-go"

	"github.com/chenen3/yeager/log"
	"github.com/chenen3/yeager/proxy/common"
)

// listener implements the net.Listener interface
type listener struct {
	ctx       context.Context
	cancelCtx context.CancelFunc
	lis       quic.Listener
	conns     chan net.Conn
}

func (l *listener) acceptLoop() {
	for {
		sess, err := l.lis.Accept(l.ctx)
		if err != nil {
			select {
			case <-l.ctx.Done():
				return
			default:
				log.Errorf("failed to accept quic session: %s", err)
				continue
			}
		}

		go l.acceptStream(sess)
	}
}

func (l *listener) acceptStream(session quic.Session) {
	defer func() {
		_ = session.CloseWithError(quic.ApplicationErrorCode(quic.NoError), "session closed")
	}()

	for {
		stream, err := session.AcceptStream(l.ctx)
		if err != nil {
			select {
			case <-l.ctx.Done():
				return
			case <-session.Context().Done():
				return
			default:
				log.Errorf("failed to accept quic stream: %s", err)
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
			localAddr:  session.LocalAddr(),
			remoteAddr: session.RemoteAddr(),
		}
	}
}

func (l *listener) Accept() (net.Conn, error) {
	select {
	case <-l.ctx.Done():
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

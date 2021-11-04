package quic

import (
	"context"
	"crypto/tls"
	"errors"
	"net"

	quic "github.com/lucas-clemente/quic-go"
	"go.uber.org/zap"
)

type Listener struct {
	ctx       context.Context
	cancelCtx context.CancelFunc
	lis       quic.Listener
	conns     chan net.Conn
}

func (l *Listener) acceptLoop() {
	for {
		sess, err := l.lis.Accept(l.ctx)
		if err != nil {
			select {
			case <-l.ctx.Done():
				return
			default:
				zap.S().Error(err)
				continue
			}
		}

		go l.acceptStream(sess)
	}
}

func (l *Listener) acceptStream(session quic.Session) {
	defer session.CloseWithError(quic.ApplicationErrorCode(quic.NoError), "session closed")
	for {
		stream, err := session.AcceptStream(l.ctx)
		if err != nil {
			select {
			case <-l.ctx.Done():
				return
			case <-session.Context().Done():
				return
			default:
				zap.S().Warn(err)
				continue
			}
		}

		select {
		case <-stream.Context().Done():
			continue
		default:
		}

		l.conns <- &conn{
			Stream:     stream,
			localAddr:  session.LocalAddr(),
			remoteAddr: session.RemoteAddr(),
		}
	}
}

func (l *Listener) Accept() (net.Conn, error) {
	select {
	case <-l.ctx.Done():
		return nil, l.ctx.Err()
	case c := <-l.conns:
		return c, nil
	}
}

func (l *Listener) Close() error {
	l.cancelCtx()
	return l.lis.Close()
}

func (l *Listener) Addr() net.Addr {
	return l.lis.Addr()
}

func Listen(addr string, tlsConf *tls.Config) (net.Listener, error) {
	if tlsConf == nil {
		return nil, errors.New("tls config required")
	}

	tlsConf.NextProtos = []string{"quic"}
	lis, err := quic.ListenAddr(addr, tlsConf, &quic.Config{KeepAlive: true})
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	l := &Listener{
		ctx:       ctx,
		cancelCtx: cancel,
		lis:       lis,
		conns:     make(chan net.Conn, 32),
	}
	go l.acceptLoop()
	return l, nil
}

package quic

import (
	"context"
	"crypto/tls"
	"net"

	quic "github.com/lucas-clemente/quic-go"
	"yeager/log"
)

type Listener struct {
	ctx       context.Context
	cancelCtx context.CancelFunc
	lis       quic.Listener
	conns     chan net.Conn
}

func (l *Listener) accept() {
	for {
		sess, err := l.lis.Accept(l.ctx)
		if err != nil {
			select {
			case <-l.ctx.Done():
				return
			default:
				log.Error(err)
				continue
			}
		}

		go l.acceptStream(sess)
	}
}

func (l *Listener) acceptStream(session quic.Session) {
	log.Debugf("session %s closed", session.LocalAddr())
	defer session.CloseWithError(quic.ErrorCode(0), "session closed")
	for {
		stream, err := session.AcceptStream(l.ctx)
		if err != nil {
			select {
			case <-l.ctx.Done():
				return
			case <-session.Context().Done():
				return
			default:
				log.Warn(err)
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
	return <-l.conns, nil
}

func (l *Listener) Close() error {
	l.cancelCtx()
	return l.lis.Close()
}

func (l *Listener) Addr() net.Addr {
	return l.lis.Addr()
}

func Listen(addr string, tlsConf *tls.Config) (net.Listener, error) {
	lis, err := quic.ListenAddr(addr, tlsConf, nil)
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
	go l.accept()
	return l, nil
}

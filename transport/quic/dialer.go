package quic

import (
	"context"
	"crypto/tls"
	"net"
	"sync"

	"github.com/lucas-clemente/quic-go"
)

type dialer struct {
	ctx       context.Context
	cancelCtx context.CancelFunc
	tlsConf   *tls.Config
	session   quic.Session
	sessionMu sync.Mutex
}

func NewDialer(tlsConf *tls.Config) *dialer {
	ctx, cancel := context.WithCancel(context.Background())
	return &dialer{ctx: ctx, cancelCtx: cancel, tlsConf: tlsConf}
}

func isValidSession(session quic.Session) bool {
	if session == nil {
		return false
	}

	select {
	case <-session.Context().Done():
		return false
	default:
		return true
	}
}

// dial a new session if no session yet or session closed
func (d *dialer) ensureValidSession(addr string) (quic.Session, error) {
	if isValidSession(d.session) {
		return d.session, nil
	}

	newSession, err := quic.DialAddrContext(d.ctx, addr, d.tlsConf, &quic.Config{KeepAlive: true})
	if err != nil {
		return nil, err
	}

	d.sessionMu.Lock()
	defer d.sessionMu.Unlock()
	// other goroutine has set the session
	if isValidSession(d.session) {
		err = newSession.CloseWithError(0, "close unused session")
		return d.session, err
	}

	d.session = newSession
	return newSession, nil
}

func (d *dialer) DialContext(ctx context.Context, addr string) (net.Conn, error) {
	session, err := d.ensureValidSession(addr)
	if err != nil {
		return nil, err
	}

	stream, err := session.OpenStreamSync(ctx)
	if err != nil {
		return nil, err
	}

	conn := &conn{
		Stream:     stream,
		localAddr:  session.LocalAddr(),
		remoteAddr: session.RemoteAddr(),
	}
	return conn, nil
}

func (d *dialer) Close() error {
	d.cancelCtx()
	return nil
}

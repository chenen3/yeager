package quic

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"sync"
	"time"

	"github.com/lucas-clemente/quic-go"
)

type dialer struct {
	tlsConf   *tls.Config
	session   quic.Session
	sessionMu sync.Mutex
}

func NewDialer(tlsConf *tls.Config) *dialer {
	return &dialer{tlsConf: tlsConf}
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

// TODO: consider saving more sessions for better throughput
// dial a new session if no session yet or session closed
func (d *dialer) quicDial(ctx context.Context, addr string) (quic.Session, error) {
	if isValidSession(d.session) {
		return d.session, nil
	}

	d.sessionMu.Lock()
	defer d.sessionMu.Unlock()
	// other goroutine has set the session
	if isValidSession(d.session) {
		return d.session, nil
	}

	qc := &quic.Config{
		MaxIdleTimeout: 30 * time.Second,
		KeepAlive:      true,
	}
	d.tlsConf.NextProtos = []string{"quic"}
	newSession, err := quic.DialAddrContext(ctx, addr, d.tlsConf, qc)
	if err != nil {
		return nil, err
	}
	d.session = newSession
	return newSession, nil
}

func (d *dialer) DialContext(ctx context.Context, network string, addr string) (net.Conn, error) {
	session, err := d.quicDial(ctx, addr)
	if err != nil {
		err = errors.New("failed to dial quic: " + err.Error())
		return nil, err
	}

	stream, err := session.OpenStreamSync(ctx)
	if err != nil {
		err = errors.New("failed to open stream: " + err.Error())
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
	if !isValidSession(d.session) {
		return nil
	}
	return d.session.CloseWithError(quic.ApplicationErrorCode(quic.NoError), "session closed")
}

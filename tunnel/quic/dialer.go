package quic

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"time"

	"github.com/lucas-clemente/quic-go"
)

type dialer struct {
	tlsConf *tls.Config
	pool    *Pool
}

// NewDialer return a QUIC dialer that implements the tunnel.Dialer interface
func NewDialer(tlsConf *tls.Config, addr string, poolSize int) *dialer {
	d := &dialer{tlsConf: tlsConf}
	dialFunc := func() (quic.Connection, error) {
		qconf := &quic.Config{
			HandshakeIdleTimeout: 5 * time.Second,
			MaxIdleTimeout:       30 * time.Second,
			KeepAlive:            true,
		}
		d.tlsConf.NextProtos = []string{"quic"}
		return quic.DialAddr(addr, d.tlsConf, qconf)
	}
	d.pool = NewPool(poolSize, dialFunc)
	return d
}

func (d *dialer) DialContext(ctx context.Context) (net.Conn, error) {
	qconn, err := d.pool.Get()
	if err != nil {
		return nil, errors.New("dial quic: " + err.Error())
	}

	stream, err := qconn.OpenStreamSync(ctx)
	if err != nil {
		return nil, errors.New("open quic stream: " + err.Error())
	}

	conn := &streamConn{
		Stream:     stream,
		localAddr:  qconn.LocalAddr(),
		remoteAddr: qconn.RemoteAddr(),
	}
	return conn, nil
}

func (d *dialer) Close() error {
	if d.pool != nil {
		return d.pool.Close()
	}
	return nil
}

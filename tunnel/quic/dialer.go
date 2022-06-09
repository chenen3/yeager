package quic

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"time"

	"github.com/lucas-clemente/quic-go"

	"github.com/chenen3/yeager/util"
)

type dialer struct {
	tlsConf *tls.Config
	pool    *Pool
}

// NewDialer return a QUIC dialer that implements the tunnel.Dialer interface
func NewDialer(tlsConf *tls.Config, addr string, poolSize int) *dialer {
	d := &dialer{tlsConf: tlsConf}
	dialFunc := func() (quic.Connection, error) {
		qc := &quic.Config{
			KeepAlive:      true,
			MaxIdleTimeout: 30 * time.Second,
		}
		d.tlsConf.NextProtos = []string{"quic"}
		ctx, cancel := context.WithTimeout(context.Background(), util.DialTimeout)
		defer cancel()
		return quic.DialAddrContext(ctx, addr, d.tlsConf, qc)
	}
	d.pool = NewPool(poolSize, dialFunc)
	return d
}

func (d *dialer) DialContext(ctx context.Context) (net.Conn, error) {
	qconn, err := d.pool.Get()
	if err != nil {
		return nil, errors.New("get quic conn: " + err.Error())
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

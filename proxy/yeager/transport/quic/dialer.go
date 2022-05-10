package quic

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"sync"
	"time"

	"github.com/chenen3/yeager/config"
	"github.com/chenen3/yeager/proxy/common"
	"github.com/lucas-clemente/quic-go"
)

type dialer struct {
	tlsConf *tls.Config
	pool    *connPool
	once    sync.Once
}

// NewDialer return a QUIC dialer that implements the transport.ContextDialer interface
func NewDialer(tlsConf *tls.Config) *dialer {
	return &dialer{tlsConf: tlsConf}
}

func (d *dialer) DialContext(ctx context.Context, addr string) (net.Conn, error) {
	d.once.Do(func() {
		factory := func() (quic.Connection, error) {
			qc := &quic.Config{
				KeepAlive:      true,
				MaxIdleTimeout: common.MaxConnectionIdle,
			}
			d.tlsConf.NextProtos = []string{"quic"}
			ctx, cancel := context.WithTimeout(context.Background(), common.DialTimeout)
			defer cancel()
			return quic.DialAddrContext(ctx, addr, d.tlsConf, qc)
		}
		d.pool = newConnPool(config.C().ConnectionPoolSize, factory)
	})

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

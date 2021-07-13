package tls

import (
	"context"
	"crypto/tls"
	"net"
	"time"
)

func Listen(addr string, tlsConf *tls.Config) (net.Listener, error) {
	return tls.Listen("tcp", addr, tlsConf)
}

type dialer struct {
	pool *ConnPool
}

func NewDialer(addr string, config *tls.Config) *dialer {
	pool := &ConnPool{
		IdleTimeout: 5 * time.Minute,
		DialContext: func(ctx context.Context) (net.Conn, error) {
			d := tls.Dialer{Config: config}
			return d.DialContext(ctx, "tcp", addr)
		},
	}
	pool.Init()
	return &dialer{pool}
}

func (d *dialer) DialContext(ctx context.Context) (net.Conn, error) {
	// 从连接池拿预先建立的连接，而不是现场发起连接，
	// 可以有效降低获取连接所需时间，改善网络体验。
	return d.pool.Get(ctx)
}

func (d *dialer) Close() error {
	return d.pool.Close()
}

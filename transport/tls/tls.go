package tls

import (
	"context"
	"crypto/tls"
	"net"
	"sync"
	"time"
)

func Listen(addr string, tlsConf *tls.Config) (net.Listener, error) {
	return tls.Listen("tcp", addr, tlsConf)
}

type dialer struct {
	config *tls.Config
	pool   *ConnPool
	once   sync.Once
}

func NewDialer(config *tls.Config) *dialer {
	return &dialer{config: config}
}

func (d *dialer) onceInitConnPool() {
	d.once.Do(func() {
		dialFunc := func(ctx context.Context, addr string) (net.Conn, error) {
			d := tls.Dialer{Config: d.config}
			return d.DialContext(ctx, "tcp", addr)
		}
		d.pool = NewPool(10, 5*time.Minute, dialFunc)
	})
}

func (d *dialer) DialContext(ctx context.Context, addr string) (net.Conn, error) {
	d.onceInitConnPool()
	// 现场建立TLS连接所需延时较大，因此从连接池获取预先建立的连接
	return d.pool.Get(ctx, addr)
}

func (d *dialer) Close() error {
	return d.pool.Close()
}

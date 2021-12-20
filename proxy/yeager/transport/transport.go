package transport

import (
	"context"
	"crypto/tls"
	"net"
)

type ContextDialer interface {
	// DialContext does not require a network argument
	// which is determined by actual implementation,
	// such as gRPC use TCP network, QUIC use UDP network.
	DialContext(ctx context.Context, addr string) (net.Conn, error)
}

type tcpDialer struct {
	dialer *net.Dialer
}

// NewTCPDialer return a TCP dialer that implements the ContextDialer interface
func NewTCPDialer() *tcpDialer {
	return &tcpDialer{dialer: new(net.Dialer)}
}

func (d *tcpDialer) DialContext(ctx context.Context, addr string) (net.Conn, error) {
	return d.dialer.DialContext(ctx, "tcp", addr)
}

type tlsDialer struct {
	dialer *tls.Dialer
}

// NewTLSDialer return a TLS dialer that implements the ContextDialer interface
func NewTLSDialer(c *tls.Config) *tlsDialer {
	return &tlsDialer{
		dialer: &tls.Dialer{
			Config: c,
		},
	}
}

func (d *tlsDialer) DialContext(ctx context.Context, addr string) (net.Conn, error) {
	return d.dialer.DialContext(ctx, "tcp", addr)
}
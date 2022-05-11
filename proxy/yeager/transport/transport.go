package transport

import (
	"context"
	"net"
)

type TunnelDialer interface {
	// DialContext does not require a network argument
	// which is determined by actual implementation,
	// such as gRPC use TCP network, QUIC use UDP network.
	DialContext(ctx context.Context) (net.Conn, error)
}

type tcpDialer struct {
	dialer *net.Dialer
	// tunnel address
	addr string
}

// NewTCPDialer return a TCP dialer that implements the TunnelDialer interface
func NewTCPDialer(addr string) *tcpDialer {
	return &tcpDialer{dialer: new(net.Dialer), addr: addr}
}

func (d *tcpDialer) DialContext(ctx context.Context) (net.Conn, error) {
	return d.dialer.DialContext(ctx, "tcp", d.addr)
}

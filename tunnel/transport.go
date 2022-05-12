package tunnel

import (
	"context"
	"net"
)

type Dialer interface {
	// DialContext does not require network nor address argument.
	// Network is determined by actual implementation,
	// such as gRPC use TCP network, QUIC use UDP network.
	// Since tunnel address is fixed, it could be pass at
	// dialer's initiation, instead of passing it every time we dial
	DialContext(ctx context.Context) (net.Conn, error)
}

type tcpDialer struct {
	dialer *net.Dialer
	// tunnel address
	addr string
}

// NewTCPDialer return a TCP dialer that implements the Dialer interface
func NewTCPDialer(addr string) *tcpDialer {
	return &tcpDialer{dialer: new(net.Dialer), addr: addr}
}

func (d *tcpDialer) DialContext(ctx context.Context) (net.Conn, error) {
	return d.dialer.DialContext(ctx, "tcp", d.addr)
}

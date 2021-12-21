package direct

import (
	"context"
	"net"
)

// Direct implements the proxy.Outbounder interface
var Direct = direct{}

type direct struct{}

func (direct) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	var d net.Dialer
	return d.DialContext(ctx, network, addr)
}

func (direct) String() string {
	return "direct"
}

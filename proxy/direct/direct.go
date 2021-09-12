package direct

import (
	"context"
	"net"
)

const Tag = "direct"

// Direct implement proxy.Outbound
var Direct = client{}

type client struct{}

func (client) DialContext(ctx context.Context, addr string) (net.Conn, error) {
	var d net.Dialer
	return d.DialContext(ctx, "tcp", addr)
}

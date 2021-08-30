package direct

import (
	"context"
	"net"
	"yeager/proxy"
)

const Tag = "direct"

type Client struct{}

func (f *Client) DialContext(ctx context.Context, addr *proxy.Address) (net.Conn, error) {
	var d net.Dialer
	return d.DialContext(ctx, "tcp", addr.String())
}

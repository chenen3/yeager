package direct

import (
	"context"
	"net"
)

const Tag = "direct"

type Client struct{}

func (f *Client) DialContext(ctx context.Context, addr string) (net.Conn, error) {
	var d net.Dialer
	return d.DialContext(ctx, "tcp", addr)
}

// TODO: 支持 UDP 拨号
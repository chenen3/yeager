package direct

import (
	"context"
	"encoding/json"
	"net"
	"yeager/proxy"
)

func init() {
	proxy.RegisterOutboundBuilder(Tag, func(json.RawMessage) (proxy.Outbound, error) {
		return new(Client), nil
	})
}

const Tag = "direct"

type Client struct{}

func (f *Client) DialContext(ctx context.Context, addr *proxy.Address) (net.Conn, error) {
	var d net.Dialer
	return d.DialContext(ctx, "tcp", addr.String())
}

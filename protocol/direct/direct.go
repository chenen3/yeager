package direct

import (
	"context"
	"encoding/json"
	"net"
	"yeager/protocol"
)

func init() {
	protocol.RegisterOutboundBuilder(Tag, func(json.RawMessage) (protocol.Outbound, error) {
		return new(Client), nil
	})
}

const Tag = "direct"

type Client struct{}

func (f *Client) DialContext(ctx context.Context, addr *protocol.Address) (net.Conn, error) {
	var d net.Dialer
	return d.DialContext(ctx, "tcp", addr.String())
}

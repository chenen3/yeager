package direct

import (
	"context"
	"encoding/json"
	"net"
	"yeager/protocol"
)

func init() {
	protocol.RegisterOutboundBuilder("direct", func(json.RawMessage) (protocol.Outbound, error) {
		return new(Client), nil
	})
}

type Client struct{}

func (f *Client) Dial(ctx context.Context, addr *protocol.Address) (net.Conn, error) {
	return net.Dial("tcp", addr.String())
}

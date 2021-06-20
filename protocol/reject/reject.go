package reject

import (
	"context"
	"encoding/json"
	"errors"
	"net"

	"yeager/protocol"
)

func init() {
	protocol.RegisterOutboundBuilder(Tag, func(json.RawMessage) (protocol.Outbound, error) {
		return new(Client), nil
	})
}

const Tag = "reject"

type Client struct{}

func (c *Client) DialContext(ctx context.Context, addr *protocol.Address) (net.Conn, error) {
	return nil, errors.New("traffic rejected")
}

package reject

import (
	"context"
	"encoding/json"
	"errors"
	"net"

	"yeager/proxy"
)

func init() {
	proxy.RegisterOutboundBuilder(Tag, func(json.RawMessage) (proxy.Outbound, error) {
		return new(Client), nil
	})
}

const Tag = "reject"

type Client struct{}

func (c *Client) DialContext(ctx context.Context, addr *proxy.Address) (net.Conn, error) {
	return nil, errors.New("traffic rejected")
}

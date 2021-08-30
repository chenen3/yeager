package reject

import (
	"context"
	"errors"
	"net"

	"yeager/proxy"
)

const Tag = "reject"

type Client struct{}

func (c *Client) DialContext(ctx context.Context, addr *proxy.Address) (net.Conn, error) {
	return nil, errors.New("traffic rejected")
}

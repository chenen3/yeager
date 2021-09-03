package reject

import (
	"context"
	"errors"
	"net"
)

const Tag = "reject"

type Client struct{}

func (c *Client) DialContext(_ context.Context, _ string) (net.Conn, error) {
	return nil, errors.New("traffic rejected")
}

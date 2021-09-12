package reject

import (
	"context"
	"errors"
	"net"
)

const Tag = "reject"

// Reject implement proxy.Outbound
var Reject = client{}

type client struct{}

func (client) DialContext(_ context.Context, _ string) (net.Conn, error) {
	return nil, errors.New("traffic rejected")
}

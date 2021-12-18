package reject

import (
	"context"
	"errors"
	"net"
)

const Tag = "reject"

// Reject implements proxy.Outbound, always reject connection and return error
var Reject = reject{}

type reject struct{}

func (reject) DialContext(_ context.Context, _, _ string) (net.Conn, error) {
	return nil, errors.New("traffic rejected")
}

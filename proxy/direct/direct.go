package direct

import (
	"context"
	"net"
)

const Tag = "direct"

// Direct implements proxy.Outbound by making network connections directly using net.DialContext
var Direct = direct{}

type direct struct{}

func (direct) DialContext(ctx context.Context, addr string) (net.Conn, error) {
	var d net.Dialer
	return d.DialContext(ctx, "tcp", addr)
}

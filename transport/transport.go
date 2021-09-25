package transport

import (
	"context"
	"net"
)

type Dialer interface {
	DialContext(ctx context.Context, network string, addr string) (net.Conn, error)
}

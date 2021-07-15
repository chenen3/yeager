package transport

import (
	"context"
	"net"
)

type Dialer interface {
	DialContext(ctx context.Context, addr string) (net.Conn, error)
}

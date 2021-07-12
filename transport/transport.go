package transport

import (
	"context"
	"net"
)

type Dialer interface {
	DialContext(ctx context.Context) (net.Conn, error)
}

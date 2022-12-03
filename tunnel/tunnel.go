package tunnel

import (
	"context"
	"io"
)

type Dialer interface {
	// DialContext connect to the ip:port target through a tunnel
	DialContext(ctx context.Context, target string) (io.ReadWriteCloser, error)
}

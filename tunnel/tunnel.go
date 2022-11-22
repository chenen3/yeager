package tunnel

import (
	"context"
	"io"
)

type Dialer interface {
	// DialContext connect to dstAddr through a tunnel
	DialContext(ctx context.Context, dstAddr string) (io.ReadWriteCloser, error)
}

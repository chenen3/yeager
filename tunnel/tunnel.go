package tunnel

import (
	"context"
	"io"
)

type Dialer interface {
	DialContext(ctx context.Context, dstAddr string) (io.ReadWriteCloser, error)
}

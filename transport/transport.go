package transport

import (
	"context"
	"io"
)

// Stream is the interface that wraps io.ReadWriteCloser and CloseWrite method
type Stream interface {
	io.ReadWriteCloser
	// close the write side of the stream, unblocking subsequent read
	CloseWrite() error
}

type Dialer interface {
	// Dial connect to the host:port address and returns stream for future read/write
	Dial(ctx context.Context, address string) (Stream, error)
}

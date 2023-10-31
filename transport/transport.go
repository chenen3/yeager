package transport

import (
	"context"
	"io"
	"net"
)

// Stream is the interface that wraps io.ReadWriteCloser and CloseWrite method
type Stream interface {
	io.ReadWriteCloser
	CloseWrite() error
}

type StreamDialer interface {
	// Dial connect to the host:port address and returns stream for future read/write
	Dial(ctx context.Context, address string) (Stream, error)
}

type TCPStreamDialer struct {
	dialer net.Dialer
}

var _ StreamDialer = (*TCPStreamDialer)(nil)

func (d *TCPStreamDialer) Dial(ctx context.Context, address string) (Stream, error) {
	conn, err := d.dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, err
	}
	return conn.(*net.TCPConn), nil
}

package transport

import (
	"context"
	"io"
	"net"
)

type Stream interface {
	io.ReadWriteCloser
	// close the write end of the stream, unblock subsequent reads
	CloseWrite() error
}

type StreamDialer interface {
	// Dial connects to the host:port address using the provided context
	Dial(ctx context.Context, address string) (Stream, error)
}

// TCPStreamDialer implements StreamDialer with the standard net.Dialer
type TCPStreamDialer struct {
	dialer net.Dialer
}

var _ StreamDialer = (*TCPStreamDialer)(nil)

func (d TCPStreamDialer) Dial(ctx context.Context, address string) (Stream, error) {
	conn, err := d.dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, err
	}
	return conn.(*net.TCPConn), nil
}

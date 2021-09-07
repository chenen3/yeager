package grpc

import (
	"net"
	"time"

	"yeager/transport/grpc/pb"
)

type streamer interface {
	Send(*pb.Data) error
	Recv() (*pb.Data, error)
}

// conn wraps grpc stream, implement net.Conn interface
type conn struct {
	stream  streamer
	buf     []byte
	off     int
	onClose func()
}

// newConn wrap grpc stream as network connection
func newConn(stream streamer, onClose func()) net.Conn {
	return &conn{
		stream:  stream,
		onClose: onClose,
	}
}

func (c *conn) Read(b []byte) (n int, err error) {
	if c.off >= len(c.buf) {
		data, err := c.stream.Recv()
		if err != nil {
			return 0, err
		}
		if data != nil {
			c.buf = data.Data
			c.off = 0
		}
	}

	n = copy(b, c.buf[c.off:])
	c.off += n
	return n, nil
}

func (c *conn) Write(b []byte) (n int, err error) {
	err = c.stream.Send(&pb.Data{Data: b})
	return len(b), err
}

func (c *conn) LocalAddr() net.Addr {
	// virtual connection does not have real IP
	addr := &net.TCPAddr{
		IP:   []byte{0, 0, 0, 0},
		Port: 0,
	}
	return addr
}

func (c *conn) RemoteAddr() net.Addr {
	// virtual connection does not have real IP
	addr := &net.TCPAddr{
		IP:   []byte{0, 0, 0, 0},
		Port: 0,
	}
	return addr
}

// SetDeadline the gRPC server already provides a connection
// idle timeout mechanism, nothing will be done here.
func (c *conn) SetDeadline(t time.Time) error {
	return nil
}

func (c *conn) SetReadDeadline(t time.Time) error {
	return nil
}

func (c *conn) SetWriteDeadline(t time.Time) error {
	return nil
}

func (c *conn) Close() error {
	if c.onClose != nil {
		defer c.onClose()
	}

	if cs, ok := c.stream.(interface{ CloseSend() error }); ok {
		// for the client-side stream
		return cs.CloseSend()
	}
	return nil
}

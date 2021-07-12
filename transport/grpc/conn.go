package grpc

import (
	"io"
	"net"
	"os"
	"time"

	"yeager/transport/grpc/pb"
)

type streamer interface {
	Send(*pb.Data) error
	Recv() (*pb.Data, error)
}

type streamConn struct {
	stream     streamer
	buf        []byte
	index      int
	done       chan struct{}
	readTimer  *time.Timer
	writeTimer *time.Timer
}

// use grpc stream to implement net.Conn
func streamToConn(stream streamer) net.Conn {
	return &streamConn{
		stream: stream,
		done:   make(chan struct{}),
	}
}

func (c *streamConn) Read(b []byte) (n int, err error) {
	select {
	case <-c.done:
		return 0, io.ErrClosedPipe
	default:
		if c.readTimer != nil {
			select {
			case <-c.readTimer.C:
				return 0, os.ErrDeadlineExceeded
			default:
			}
		}
	}

	if c.index >= len(c.buf) {
		data, err := c.stream.Recv()
		if err != nil {
			return 0, err
		}
		if data != nil {
			c.buf = data.Data
			c.index = 0
		}
	}

	n = copy(b, c.buf[c.index:])
	c.index += n
	return n, nil
}

func (c *streamConn) Write(b []byte) (n int, err error) {
	select {
	case <-c.done:
		return 0, io.ErrClosedPipe
	default:
		if c.writeTimer != nil {
			select {
			case <-c.writeTimer.C:
				return 0, os.ErrDeadlineExceeded
			default:
			}
		}
	}

	err = c.stream.Send(&pb.Data{Data: b})
	return len(b), err
}

func (c *streamConn) Close() error {
	close(c.done)
	if c.writeTimer != nil {
		c.writeTimer.Stop()
	}
	if c.readTimer != nil {
		c.readTimer.Stop()
	}
	if cs, ok := c.stream.(interface{ CloseSend() error }); ok {
		// for the client-side stream
		return cs.CloseSend()
	}
	return nil
}

func (c *streamConn) LocalAddr() net.Addr {
	addr := &net.TCPAddr{
		IP:   []byte{0, 0, 0, 0},
		Port: 0,
	}
	return addr
}

func (c *streamConn) RemoteAddr() net.Addr {
	addr := &net.TCPAddr{
		IP:   []byte{0, 0, 0, 0},
		Port: 0,
	}
	return addr
}

func (c *streamConn) SetDeadline(t time.Time) error {
	err := c.SetReadDeadline(t)
	if err != nil {
		return err
	}
	return c.SetWriteDeadline(t)
}

func (c *streamConn) SetReadDeadline(t time.Time) error {
	if c.readTimer == nil {
		c.readTimer = time.NewTimer(time.Until(t))
		return nil
	}

	if !c.readTimer.Stop() {
		<-c.readTimer.C
	}
	c.readTimer.Reset(time.Until(t))
	return nil
}

func (c *streamConn) SetWriteDeadline(t time.Time) error {
	if c.writeTimer == nil {
		c.writeTimer = time.NewTimer(time.Until(t))
		return nil
	}

	if !c.writeTimer.Stop() {
		<-c.writeTimer.C
	}
	c.writeTimer.Reset(time.Until(t))
	return nil
}

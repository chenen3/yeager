package grpc

import (
	"net"
	"time"

	"google.golang.org/grpc/peer"

	"github.com/chenen3/yeager/tunnel/grpc/pb"
)

// serverStreamConn wraps grpc server-side stream, implement net.Conn interface
type serverStreamConn struct {
	stream     pb.Tunnel_StreamServer
	onClose    func()
	buf        []byte
	off        int
	localAddr  net.Addr
	remoteAddr net.Addr
}

func serverStreamAsConn(stream pb.Tunnel_StreamServer, onClose func()) *serverStreamConn {
	conn := serverStreamConn{stream: stream, onClose: onClose}
	conn.localAddr = &net.TCPAddr{IP: []byte{0, 0, 0, 0}, Port: 0}
	p, ok := peer.FromContext(conn.stream.Context())
	if ok {
		conn.remoteAddr = p.Addr
	} else {
		conn.remoteAddr = &net.TCPAddr{IP: []byte{0, 0, 0, 0}, Port: 0}
	}
	return &conn
}

func (c *serverStreamConn) Read(b []byte) (n int, err error) {
	if c.off >= len(c.buf) {
		data, err := c.stream.Recv()
		if err != nil {
			return 0, err
		}
		c.buf = data.Data
		c.off = 0
	}
	n = copy(b, c.buf[c.off:])
	c.off += n
	return n, nil
}

func (c *serverStreamConn) Write(b []byte) (n int, err error) {
	err = c.stream.Send(&pb.Data{Data: b})
	return len(b), err
}

func (c *serverStreamConn) LocalAddr() net.Addr {
	return c.localAddr
}

func (c *serverStreamConn) RemoteAddr() net.Addr {
	return c.remoteAddr
}

func (c *serverStreamConn) SetDeadline(t time.Time) error {
	return nil
}

func (c *serverStreamConn) SetReadDeadline(t time.Time) error {
	return nil
}

func (c *serverStreamConn) SetWriteDeadline(t time.Time) error {
	return nil
}

func (c *serverStreamConn) Close() error {
	c.onClose()
	return nil
}

// clientStreamConn wraps grpc client-side stream, implement net.Conn interface
type clientStreamConn struct {
	stream     pb.Tunnel_StreamClient
	onClose    func()
	buf        []byte
	off        int
	localAddr  net.Addr
	remoteAddr net.Addr
}

func clientStreamAsConn(stream pb.Tunnel_StreamClient, onClose func()) *clientStreamConn {
	conn := clientStreamConn{stream: stream, onClose: onClose}
	conn.localAddr = &net.TCPAddr{IP: []byte{0, 0, 0, 0}, Port: 0}
	conn.remoteAddr = &net.TCPAddr{IP: []byte{0, 0, 0, 0}, Port: 0}
	return &conn
}

func (c *clientStreamConn) Read(b []byte) (n int, err error) {
	if c.off >= len(c.buf) {
		data, err := c.stream.Recv()
		if err != nil {
			return 0, err
		}
		c.buf = data.Data
		c.off = 0
	}
	n = copy(b, c.buf[c.off:])
	c.off += n
	return n, nil
}

// Write wraps grpc client-side stream Send(), which is SendMsg() actually.
// according to grpc doc:
//
//	SendMsg does not wait until the message is received by the server. An
//	untimely stream closure may result in lost messages. To ensure delivery,
//	users should ensure the RPC completed successfully using RecvMsg.
//
// back to clientStreamConn, once we've finished all the Write(), we need
// to call CloseSend(), and wait for response from Read(), then Close()
func (c *clientStreamConn) Write(b []byte) (n int, err error) {
	err = c.stream.Send(&pb.Data{Data: b})
	return len(b), err
}

func (c *clientStreamConn) LocalAddr() net.Addr {
	return c.localAddr
}

func (c *clientStreamConn) RemoteAddr() net.Addr {
	return c.remoteAddr
}

func (c *clientStreamConn) SetDeadline(t time.Time) error {
	return nil
}

func (c *clientStreamConn) SetReadDeadline(t time.Time) error {
	return nil
}

func (c *clientStreamConn) SetWriteDeadline(t time.Time) error {
	return nil
}

// Close cancel grpc stream context, whose CancelFunc
// is passed to onClose in initialization
func (c *clientStreamConn) Close() error {
	if c.onClose != nil {
		c.onClose()
	}
	return nil
}

func (c *clientStreamConn) CloseSend() error {
	return c.stream.CloseSend()
}

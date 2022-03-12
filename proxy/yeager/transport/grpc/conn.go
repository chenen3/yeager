package grpc

import (
	"context"
	"io"
	"net"
	"os"
	"time"

	"google.golang.org/grpc/peer"

	"github.com/chenen3/yeager/proxy/yeager/transport/grpc/pb"
)

// serverStreamConn wraps grpc server-side stream, implement net.Conn interface
type serverStreamConn struct {
	stream     pb.Tunnel_StreamServer
	onClose    func()
	buf        []byte
	off        int
	localAddr  net.Addr
	remoteAddr net.Addr

	readCtx       context.Context
	readCtxCancel context.CancelFunc
}

func serverStreamToConn(stream pb.Tunnel_StreamServer, onClose func()) *serverStreamConn {
	conn := serverStreamConn{stream: stream, onClose: onClose}
	conn.localAddr = &net.TCPAddr{IP: []byte{0, 0, 0, 0}, Port: 0}
	p, ok := peer.FromContext(conn.stream.Context())
	if ok {
		conn.remoteAddr = p.Addr
	} else {
		conn.remoteAddr = &net.TCPAddr{IP: []byte{0, 0, 0, 0}, Port: 0}
	}

	conn.readCtx, conn.readCtxCancel = context.WithCancel(context.Background())
	return &conn
}

type readResult struct {
	byteCount int
	err       error
}

func (c *serverStreamConn) Read(b []byte) (n int, err error) {
	results := make(chan readResult, 1)
	go func() {
		if c.off >= len(c.buf) {
			data, err := c.stream.Recv()
			if err != nil {
				results <- readResult{err: err}
				return
			}
			c.buf = data.Data
			c.off = 0
		}
		n = copy(b, c.buf[c.off:])
		c.off += n
		results <- readResult{byteCount: n}
	}()

	select {
	case <-c.readCtx.Done():
		return 0, os.ErrDeadlineExceeded
	case result := <-results:
		return result.byteCount, result.err
	}
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
	var ctx context.Context
	var cancel context.CancelFunc
	if t.IsZero() {
		ctx, cancel = context.WithCancel(context.Background())
	} else {
		ctx, cancel = context.WithDeadline(context.Background(), t)
	}
	c.readCtx, ctx = ctx, c.readCtx
	c.readCtxCancel, cancel = cancel, c.readCtxCancel
	_ = ctx
	cancel()
	return nil
}

func (c *serverStreamConn) SetWriteDeadline(t time.Time) error {
	return nil
}

func (c *serverStreamConn) Close() error {
	c.readCtxCancel()
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

	readCtx       context.Context
	readCtxCancel context.CancelFunc
}

func clientStreamToConn(stream pb.Tunnel_StreamClient, onClose func()) *clientStreamConn {
	conn := clientStreamConn{stream: stream, onClose: onClose}
	conn.localAddr = &net.TCPAddr{IP: []byte{0, 0, 0, 0}, Port: 0}
	conn.remoteAddr = &net.TCPAddr{IP: []byte{0, 0, 0, 0}, Port: 0}
	conn.readCtx, conn.readCtxCancel = context.WithCancel(context.Background())
	return &conn
}

func (c *clientStreamConn) Read(b []byte) (n int, err error) {
	results := make(chan readResult, 1)
	go func() {
		if c.off >= len(c.buf) {
			data, err := c.stream.Recv()
			if err != nil {
				if err == io.EOF {
					// client-side SendMsg does not wait until the message is received by the server. An
					// untimely stream closure may result in lost messages. To ensure delivery,
					// users should ensure the RPC completed successfully using RecvMsg.
					c.onClose()
				}
				results <- readResult{err: err}
				return
			}
			c.buf = data.Data
			c.off = 0
		}
		n = copy(b, c.buf[c.off:])
		c.off += n
		results <- readResult{byteCount: n}
	}()

	select {
	case <-c.readCtx.Done():
		return 0, os.ErrDeadlineExceeded
	case result := <-results:
		return result.byteCount, result.err
	}
}

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
	var ctx context.Context
	var cancel context.CancelFunc
	if t.IsZero() {
		ctx, cancel = context.WithCancel(context.Background())
	} else {
		ctx, cancel = context.WithDeadline(context.Background(), t)
	}
	c.readCtx, ctx = ctx, c.readCtx
	c.readCtxCancel, cancel = cancel, c.readCtxCancel
	_ = ctx
	cancel()
	return nil
}

func (c *clientStreamConn) SetWriteDeadline(t time.Time) error {
	return nil
}

func (c *clientStreamConn) Close() error {
	c.readCtxCancel()
	return c.stream.CloseSend()
}

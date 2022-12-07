package quic

import (
	"context"
	"net"
	"testing"

	"github.com/lucas-clemente/quic-go"
)

// conn implements the quic.Connection interface
type conn struct {
	ctx    context.Context
	cancel context.CancelFunc
}

func fakeQuicConn() *conn {
	ctx, cancel := context.WithCancel(context.Background())
	return &conn{ctx, cancel}
}

func (c *conn) AcceptStream(context.Context) (quic.Stream, error) {
	panic("to be implement")
}

func (c *conn) AcceptUniStream(context.Context) (quic.ReceiveStream, error) {
	panic("to be implement")
}

func (c *conn) OpenStream() (quic.Stream, error) {
	panic("to be implement")
}

func (c *conn) OpenStreamSync(context.Context) (quic.Stream, error) {
	panic("to be implement")
}

func (c *conn) OpenUniStream() (quic.SendStream, error) {
	panic("to be implement")
}

func (c *conn) OpenUniStreamSync(context.Context) (quic.SendStream, error) {
	panic("to be implement")
}

func (c *conn) LocalAddr() net.Addr {
	panic("to be implement")
}

func (c *conn) RemoteAddr() net.Addr {
	panic("to be implement")
}

func (c *conn) CloseWithError(quic.ApplicationErrorCode, string) error {
	c.cancel()
	return nil
}

func (c *conn) Context() context.Context {
	return c.ctx
}

func (c *conn) ConnectionState() quic.ConnectionState {
	panic("to be implement")
}

func (c *conn) SendMessage([]byte) error {
	panic("to be implement")
}

func (c *conn) ReceiveMessage() ([]byte, error) {
	panic("to be implement")
}

func TestPoolGet(t *testing.T) {
	p := newConnPool(2, func() (quic.Connection, error) {
		return fakeQuicConn(), nil
	})
	defer p.Close()

	conn, err := p.Get()
	if err != nil {
		t.Error(err)
		return
	}
	defer conn.CloseWithError(0, "")
	if !isValid(conn) {
		t.Error("dead connection")
		return
	}
}

func TestReconnectParallel(t *testing.T) {
	p := newConnPool(2, func() (quic.Connection, error) {
		return fakeQuicConn(), nil
	})
	defer p.Close()

	t.Run("group", func(t *testing.T) {
		parallelTest := func(t *testing.T) {
			t.Parallel()
			conn, err := p.Get()
			if err != nil {
				t.Error(err)
				return
			}
			defer conn.CloseWithError(0, "")
			if !isValid(conn) {
				t.Error("dead connection")
				return
			}
		}
		t.Run("test1", parallelTest)
		t.Run("test2", parallelTest)
		t.Run("test3", parallelTest)
	})
}

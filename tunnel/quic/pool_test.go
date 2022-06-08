package quic

import (
	"context"
	"net"
	"testing"

	"github.com/lucas-clemente/quic-go"
)

// fakeQuicConn implements the quic.Connection interface
type fakeQuicConn struct {
	ctx    context.Context
	cancel context.CancelFunc
}

func newFakeQuicConn() *fakeQuicConn {
	ctx, cancel := context.WithCancel(context.Background())
	return &fakeQuicConn{ctx, cancel}
}

func (c *fakeQuicConn) AcceptStream(context.Context) (quic.Stream, error) {
	panic("to be implement")
}

func (c *fakeQuicConn) AcceptUniStream(context.Context) (quic.ReceiveStream, error) {
	panic("to be implement")
}

func (c *fakeQuicConn) OpenStream() (quic.Stream, error) {
	panic("to be implement")
}

func (c *fakeQuicConn) OpenStreamSync(context.Context) (quic.Stream, error) {
	panic("to be implement")
}

func (c *fakeQuicConn) OpenUniStream() (quic.SendStream, error) {
	panic("to be implement")
}

func (c *fakeQuicConn) OpenUniStreamSync(context.Context) (quic.SendStream, error) {
	panic("to be implement")
}

func (c *fakeQuicConn) LocalAddr() net.Addr {
	panic("to be implement")
}

func (c *fakeQuicConn) RemoteAddr() net.Addr {
	panic("to be implement")
}

func (c *fakeQuicConn) CloseWithError(quic.ApplicationErrorCode, string) error {
	c.cancel()
	return nil
}

func (c *fakeQuicConn) Context() context.Context {
	return c.ctx
}

func (c *fakeQuicConn) ConnectionState() quic.ConnectionState {
	panic("to be implement")
}

func (c *fakeQuicConn) SendMessage([]byte) error {
	panic("to be implement")
}

func (c *fakeQuicConn) ReceiveMessage() ([]byte, error) {
	panic("to be implement")
}

func TestPoolGet(t *testing.T) {
	makeConn := func() (quic.Connection, error) {
		return newFakeQuicConn(), nil
	}
	p := NewConnPool(2, makeConn)
	defer p.Close()

	conn, err := p.Get()
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseWithError(0, "")
	if !isValid(conn) {
		t.Fatal("dead connection")
	}
}

func TestPoolReconnect(t *testing.T) {
	p := NewConnPool(2, func() (quic.Connection, error) {
		return newFakeQuicConn(), nil
	})
	defer p.Close()

	for _, conn := range p.conns {
		conn.CloseWithError(0, "")
	}
	conn, err := p.Get()
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseWithError(0, "")
	if !isValid(conn) {
		t.Fatal("dead connection")
	}
}

package quic

import (
	"context"
	"net"
	"sync"
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
	dialFunc := func() (quic.Connection, error) {
		return fakeQuicConn(), nil
	}
	p := NewPool(2, dialFunc)
	defer p.Close()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
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
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
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
	}()
	wg.Wait()
}

func TestPoolReconnect(t *testing.T) {
	p := NewPool(2, func() (quic.Connection, error) {
		return fakeQuicConn(), nil
	})
	defer p.Close()

	for _, conn := range p.conns {
		conn.CloseWithError(0, "")
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
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
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
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
	}()
	wg.Wait()
}

package util

import (
	"context"
	"net"
	"testing"
	"time"
	"yeager/log"
)

func TestConnPool(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		conn, err := l.Accept()
		if err != nil {
			log.Error()
			return
		}
		conn.Close()
	}()

	dialContext := func(ctx context.Context) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, l.Addr().Network(), l.Addr().String())
	}
	p := ConnPool{DialContext: dialContext, IdleTimeout: time.Millisecond}
	p.Init()
	defer p.Close()

	ctx, cancel := context.WithTimeout(context.Background(),time.Second)
	defer cancel()
	conn, err := p.Get(ctx)
	if err != nil {
		t.Fatal(err)
	}
	conn.Close()

	// wait for idle timeout
	time.Sleep(2 * time.Millisecond)
	conn, err = p.Get(ctx)
	if err != nil {
		t.Fatal(err)
	}
	conn.Close()
}
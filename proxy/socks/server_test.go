package socks

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"testing"
	"time"

	gproxy "golang.org/x/net/proxy"

	"github.com/chenen3/yeager/log"
	"github.com/chenen3/yeager/util"
)

func TestServer(t *testing.T) {
	port, err := util.ChoosePort()
	if err != nil {
		t.Fatal(err)
	}

	srv, err := NewServer(fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	srv.Handle(func(ctx context.Context, conn net.Conn, addr string) {
		defer conn.Close()
		if addr != "fake.domain.com:1234" {
			t.Errorf("received unexpected dst addr: %s", addr)
			return
		}
		_, _ = io.Copy(conn, conn)
	})
	go func() {
		e := srv.ListenAndServe()
		if e != nil {
			log.Errorf("server exit: %s", err)
		}
	}()

	select {
	case <-time.After(time.Second):
		t.Fatal("server not ready in time")
	case <-srv.ready:
	}

	client, err := gproxy.SOCKS5("tcp", srv.addr, nil, nil)
	if err != nil {
		t.Fatal(err)
		return
	}
	conn, err := client.Dial("tcp", "fake.domain.com:1234")
	if err != nil {
		t.Fatal(err)
		return
	}
	defer conn.Close()

	want := []byte("1")
	_, err = conn.Write(want)
	if err != nil {
		t.Fatal(err)
	}
	got := make([]byte, 1)
	_, err = io.ReadFull(conn, got)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(want, got) {
		t.Fatalf("want %v, got %v", want, got)
	}
}

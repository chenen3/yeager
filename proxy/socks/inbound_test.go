package socks

import (
	"bytes"
	"context"
	"io"
	"net"
	"strconv"
	"testing"

	gproxy "golang.org/x/net/proxy"
	"yeager/proxy"
	"yeager/util"
)

func TestServer(t *testing.T) {
	port, err := util.ChoosePort()
	if err != nil {
		t.Fatal(err)
	}

	server := NewServer(&Config{
		Host: "127.0.0.1",
		Port: port,
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		t.Log(server.ListenAndServe(ctx))
	}()
	server.RegisterHandler(func(ctx context.Context, conn net.Conn, addr *proxy.Address) {
		defer conn.Close()
		if addr.String() != "fake.domain.com:1234" {
			t.Errorf("received unexpected dst addr: %s", addr.String())
			return
		}
		io.Copy(conn, conn)
	})

	<-server.ready
	addr := net.JoinHostPort(server.conf.Host, strconv.Itoa(server.conf.Port))
	client, err := gproxy.SOCKS5("tcp", addr, nil, nil)
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
	io.ReadFull(conn, got)
	if !bytes.Equal(want, got) {
		t.Fatalf("want %v, got %v", want, got)
	}
}

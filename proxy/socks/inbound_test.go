package socks

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"testing"
	"yeager/config"

	gproxy "golang.org/x/net/proxy"
	"yeager/proxy"
	"yeager/util"
)

func TestServer(t *testing.T) {
	port, err := util.ChoosePort()
	if err != nil {
		t.Fatal(err)
	}

	server := NewServer(&config.SOCKSServerConfig{
		Address: fmt.Sprintf("127.0.0.1:%d", port),
	})
	defer server.Close()
	go func() {
		err := server.ListenAndServe(func(ctx context.Context, conn net.Conn, addr *proxy.Address) {
			defer conn.Close()
			if addr.String() != "fake.domain.com:1234" {
				t.Errorf("received unexpected dst addr: %s", addr.String())
				return
			}
			io.Copy(conn, conn)
		})
		if err != nil {
			t.Error(err)
		}
	}()

	<-server.ready
	client, err := gproxy.SOCKS5("tcp", server.conf.Address, nil, nil)
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

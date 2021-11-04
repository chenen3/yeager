package socks

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"testing"

	gproxy "golang.org/x/net/proxy"

	"github.com/chenen3/yeager/config"
	"github.com/chenen3/yeager/util"
)

func TestServer(t *testing.T) {
	port, err := util.ChoosePort()
	if err != nil {
		t.Fatal(err)
	}

	server, err := NewServer(&config.SOCKSProxy{
		Listen: fmt.Sprintf("127.0.0.1:%d", port),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	go func() {
		err := server.ListenAndServe(func(ctx context.Context, conn net.Conn, addr string) {
			defer conn.Close()
			if addr != "fake.domain.com:1234" {
				t.Errorf("received unexpected dst addr: %s", addr)
				return
			}
			io.Copy(conn, conn)
		})
		if err != nil {
			t.Error(err)
		}
	}()

	<-server.ready
	client, err := gproxy.SOCKS5("tcp", server.conf.Listen, nil, nil)
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

package socks

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"testing"

	"github.com/chenen3/yeager/config"
	"github.com/chenen3/yeager/util"
)

func TestUDPServer(t *testing.T) {
	port, err := util.ChoosePort()
	if err != nil {
		t.Fatal(err)
	}

	tcpServer, err := NewTCPServer(&config.SOCKSProxy{
		Listen: fmt.Sprintf("127.0.0.1:%d", port),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer tcpServer.Close()
	go func() {
		// tcp handler is not required in udp test
		tcpServer.ListenAndServe(nil)
	}()

	udpServer, err := NewUDPServer(&config.SOCKSProxy{
		Listen: fmt.Sprintf("127.0.0.1:%d", port),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer udpServer.Close()
	go func() {
		err := udpServer.ListenAndServe(func(ctx context.Context, conn net.Conn, network, addr string) {
			defer conn.Close()
			if addr != "fake.domain.com:1234" {
				t.Errorf("received unexpected addr: %s", addr)
				return
			}
			io.Copy(conn, conn)
		})
		if err != nil {
			t.Error(err)
		}
	}()

	<-tcpServer.ready
	<-udpServer.ready
	d := dialer{ServerAddr: udpServer.conf.Listen}
	conn, err := d.Dial("udp", "fake.domain.com:1234")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	want := []byte("1")
	if _, err = conn.Write(want); err != nil {
		t.Fatal(err)
	}
	got := make([]byte, 1)
	if _, err = io.ReadFull(conn, got); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(want, got) {
		t.Fatalf("want %s, got %s", want, got)
	}
}

func TestDatagram(t *testing.T) {
	dst, err := util.ParseAddress("127.0.0.1:80")
	if err != nil {
		t.Fatal(err)
	}

	wantDg := &datagram{dst: dst, data: []byte{1}}
	bs, err := wantDg.Marshal()
	if err != nil {
		t.Fatal(err)
	}

	var gotDg datagram
	if err := gotDg.Unmarshal(bs); err != nil {
		t.Fatal(err)
	}

	if dst.String() != gotDg.dst.String() {
		t.Fatalf("want %s, got %s", dst, gotDg.dst)
	}
}

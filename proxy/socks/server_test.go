package socks

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"testing"

	gproxy "golang.org/x/net/proxy"
	"yeager/config"
	"yeager/proxy"
	"yeager/util"
)

func TestServer(t *testing.T) {
	port, err := util.ChoosePort()
	if err != nil {
		t.Fatal(err)
	}

	server := NewServer(&config.SOCKSProxy{
		Address: fmt.Sprintf("127.0.0.1:%d", port),
	})
	defer server.Close()
	go func() {
		err := server.ListenAndServe(func(ctx context.Context, conn net.Conn, addr *proxy.Address) {
			defer conn.Close()
			if addr.String() != "fake.domain.com:1234" {
				t.Errorf("received unexpected addr addr: %s", addr)
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

func TestUDPServer(t *testing.T) {
	port, err := util.ChoosePort()
	if err != nil {
		t.Fatal(err)
	}

	tcpServer := NewServer(&config.SOCKSProxy{
		Address: fmt.Sprintf("127.0.0.1:%d", port),
	})
	defer tcpServer.Close()
	go func() {
		err := tcpServer.ListenAndServe(func(ctx context.Context, conn net.Conn, addr *proxy.Address) {
			defer conn.Close()
			if addr.String() != "fake.domain.com:1234" {
				t.Errorf("received unexpected addr addr: %s", addr)
				return
			}
			io.Copy(conn, conn)
		})
		if err != nil {
			t.Error(err)
		}
	}()

	udpServer := NewUDPServer(&config.SOCKSProxy{
		Address: fmt.Sprintf("127.0.0.1:%d", port),
	})
	defer udpServer.Close()
	go func() {
		err := udpServer.ListenAndServe(func(ctx context.Context, conn net.Conn, addr *proxy.Address) {
			defer conn.Close()
			if addr.String() != "fake.domain.com:1234" {
				t.Errorf("received unexpected addr addr: %s", addr)
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
	d := dialer{proxyAddress: udpServer.conf.Address}
	conn, err := d.dial("udp", "fake.domain.com:1234")
	if err != nil {
		t.Fatal(err)
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

func TestDatagram(t *testing.T) {
	dst, err := proxy.ParseAddress("udp", "127.0.0.1:80")
	if err != nil {
		t.Fatal(err)
	}
	bs := marshalDatagram(dst, []byte{1})

	dg, err := parseDatagram(bs)
	if err != nil {
		t.Fatal(err)
	}

	if dg.dst.String() != dst.String() {
		t.Fatalf("want %s, got %s", dst, dg.dst)
	}
}

package socks

import (
	"golang.org/x/net/proxy"
	"net"
	"strconv"
	"testing"
	"time"
	"yeager/util"
)

func TestSocks5(t *testing.T) {
	port, err := util.ChoosePort()
	if err != nil {
		t.Fatal(err)
	}
	ss := NewServer(&Config{
		Host: "127.0.0.1",
		Port: port,
	})
	go ss.Serve()
	defer ss.Close()
	time.Sleep(time.Millisecond)

	addr := net.JoinHostPort(ss.conf.Host, strconv.Itoa(ss.conf.Port))
	client, err := proxy.SOCKS5("tcp", addr, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	// test connect operation
	c, err := client.Dial("tcp", "127.0.0.1:1234")
	if err != nil {
		t.Fatal(err)
	}
	c.Close()
}

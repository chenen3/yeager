package socks

import (
	"net"
	"strconv"
	"testing"
	"time"

	"golang.org/x/net/proxy"
	"yeager/util"
)

func TestServer(t *testing.T) {
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

	go func() {
		addr := net.JoinHostPort(ss.conf.Host, strconv.Itoa(ss.conf.Port))
		client, err := proxy.SOCKS5("tcp", addr, nil, nil)
		if err != nil {
			t.Log(err)
			return
		}
		// waiting the proxy server start up
		time.Sleep(time.Millisecond)
		c, err := client.Dial("tcp", "1.2.3.4:80")
		if err != nil {
			t.Log(err)
			return
		}
		c.Close()
	}()

	conn := <-ss.Accept()
	defer conn.Close()
	if dst := conn.DstAddr().Host; dst != "1.2.3.4" {
		t.Fatalf("proxy server got unexpected destination address: %s", dst)
	}
}

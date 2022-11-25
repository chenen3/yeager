package socks

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	ynet "github.com/chenen3/yeager/net"
)

type direct struct{}

func (d direct) DialContext(ctx context.Context, addr string) (io.ReadWriteCloser, error) {
	var dialer net.Dialer
	return dialer.DialContext(ctx, "tcp", addr)
}

func TestSocksProxy(t *testing.T) {
	want := "ok"
	httpSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, want)
	}))
	defer httpSrv.Close()

	port, _ := ynet.AllocatePort()
	address := fmt.Sprintf("127.0.0.1:%d", port)
	ready := make(chan struct{})
	var s Server
	defer s.Close()
	go func() {
		close(ready)
		s.Serve(address, direct{})
	}()

	pu, err := url.Parse("socks5://" + address)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyURL(pu),
		},
		Timeout: time.Second,
	}
	<-ready
	// the proxy server may not started yet
	time.Sleep(time.Millisecond)
	resp, err := client.Get(httpSrv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("bad status: %s", resp.Status)
	}
	got, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("want %s, got %s", want, got)
	}
}

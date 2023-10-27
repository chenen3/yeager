package proxy

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

var directDial = func(ctx context.Context, network, address string) (io.ReadWriteCloser, error) {
	var d net.Dialer
	return d.DialContext(ctx, network, address)
}

func TestSocksProxy(t *testing.T) {
	want := "ok"
	httpSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(want))
	}))
	defer httpSrv.Close()

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ready := make(chan struct{})
	s := NewSOCKS5Server(directDial)
	defer s.Close()
	go func() {
		close(ready)
		if e := s.Serve(lis); e != nil {
			t.Log(e)
		}
	}()

	u, err := url.Parse("socks5://" + lis.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyURL(u),
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
		t.Fatalf("got %s, want %s", got, want)
	}
}

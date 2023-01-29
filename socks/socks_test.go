package socks

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

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ready := make(chan struct{})
	s := NewServer()
	defer s.Close()
	go func() {
		close(ready)
		if e := s.Serve(lis, direct{}); e != nil {
			t.Log(e)
		}
	}()

	pu, err := url.Parse("socks5://" + lis.Addr().String())
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
		t.Fatalf("got %s, want %s", got, want)
	}
}

func TestServer_Close(t *testing.T) {
	// test no-op Close
	s := NewServer()
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	// test if Serve can exit properly when Close called
	s = NewServer()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ready := make(chan struct{})
	go func() {
		<-ready
		// in case Serve has not started yet
		time.Sleep(time.Millisecond)
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	}()
	close(ready)
	if err := s.Serve(lis, direct{}); err != nil {
		t.Fatal(err)
	}
}

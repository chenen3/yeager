package httpproxy

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

func TestHttpProxy(t *testing.T) {
	ok := "ok"
	httpSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, ok)
	}))
	defer httpSrv.Close()

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ready := make(chan struct{})
	var s Server
	defer s.Close()
	go func() {
		close(ready)
		s.Serve(lis, direct{})
	}()

	proxyUrl, _ := url.Parse("http://" + lis.Addr().String())
	client := &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyURL(proxyUrl),
			DialContext: (&net.Dialer{
				Timeout: 5 * time.Second,
			}).DialContext,
		},
	}
	<-ready
	// the proxy server may not started yet
	time.Sleep(time.Millisecond)
	res, err := client.Get(httpSrv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	got, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != ok {
		t.Fatalf("want %s, got %s", ok, got)
	}
}

func TestHttpsProxy(t *testing.T) {
	ok := "ok"
	httpsSrv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, ok)
	}))
	defer httpsSrv.Close()

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var s Server
	defer s.Close()
	ready := make(chan struct{})
	go func() {
		close(ready)
		s.Serve(lis, direct{})
	}()

	proxyUrl, _ := url.Parse("http://" + lis.Addr().String())
	client := httpsSrv.Client()
	tr := client.Transport.(*http.Transport)
	tr = tr.Clone()
	tr.Proxy = http.ProxyURL(proxyUrl)
	client.Transport = tr
	<-ready
	// the proxy server may not started yet
	time.Sleep(time.Millisecond)
	res, err := client.Get(httpsSrv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	got, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != ok {
		t.Fatalf("want %s, got %s", ok, got)
	}
}

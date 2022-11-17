package httpproxy

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

	"github.com/chenen3/yeager/util"
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

	port, _ := util.AllocatePort()
	proxyAddr := fmt.Sprintf("127.0.0.1:%d", port)
	ready := make(chan struct{})
	var s Server
	defer s.Close()
	go func() {
		close(ready)
		s.Serve(proxyAddr, direct{})
	}()

	proxyUrl, _ := url.Parse("http://" + proxyAddr)
	client := &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyURL(proxyUrl),
			DialContext: (&net.Dialer{
				Timeout: 5 * time.Second,
			}).DialContext,
		},
	}
	<-ready
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

	port, _ := util.AllocatePort()
	proxyAddr := fmt.Sprintf("127.0.0.1:%d", port)
	var s Server
	defer s.Close()
	ready := make(chan struct{})
	go func() {
		close(ready)
		s.Serve(proxyAddr, direct{})
	}()

	proxyUrl, _ := url.Parse("http://" + proxyAddr)
	client := httpsSrv.Client()
	tr := client.Transport.(*http.Transport)
	tr = tr.Clone()
	tr.Proxy = http.ProxyURL(proxyUrl)
	client.Transport = tr
	<-ready
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

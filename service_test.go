package main

import (
	"context"
	"crypto/tls"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/chenen3/yeager/echo"
)

func TestHttpProxyToGRPC(t *testing.T) {
	cc, sc, err := GenerateConfig("127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	stop, err := start(cc)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()

	stop, err = start(sc)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "1")
	}))
	defer ts.Close()

	pu, err := url.Parse("http://" + cc.ListenHTTP)
	if err != nil {
		t.Fatal(err)
	}
	client := http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyURL(pu),
		},
		Timeout: time.Second,
	}

	// the proxy services may not started yet
	time.Sleep(time.Millisecond)

	// traffic flows: client request -> http proxy -> tunnel client -> tunnel server -> http test server
	resp, err := client.Get(ts.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	bs, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(bs) != "1" {
		t.Fatalf("want 1, got %s", bs)
	}
}

func TestHttpsProxyToHTTP2(t *testing.T) {
	cc, sc, err := GenerateConfig("127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}

	cc.Proxy.Proto = ProtoHTTP2
	stop, err := start(cc)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()

	sc.Listen[0].Proto = ProtoHTTP2
	stop, err = start(sc)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()

	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "1")
	}))
	defer ts.Close()

	pu, err := url.Parse("http://" + cc.ListenHTTP)
	if err != nil {
		t.Fatal(err)
	}
	client := http.Client{
		Transport: &http.Transport{
			Proxy:           http.ProxyURL(pu),
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
		Timeout: time.Second,
	}

	// the proxy services may not started yet
	time.Sleep(time.Millisecond)
	resp, err := client.Get(ts.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	bs, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(bs) != "1" {
		t.Fatalf("want 1, got %s", bs)
	}
}

func TestSocksProxyToGRPC(t *testing.T) {
	cc, sc, err := GenerateConfig("127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	stop, err := start(cc)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	stop, err = start(sc)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "1")
	}))
	defer ts.Close()

	pu, err := url.Parse("socks5://" + cc.ListenSOCKS)
	if err != nil {
		t.Fatal(err)
	}
	client := http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyURL(pu),
		},
		Timeout: time.Second,
	}

	// the proxy services may not started yet
	time.Sleep(time.Millisecond)

	// traffic flows: client request -> socks proxy -> tunnel client -> tunnel server -> http test server
	resp, err := client.Get(ts.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	bs, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(bs) != "1" {
		t.Fatalf("want 1, got %s", bs)
	}
}

func TestDialPrivate(t *testing.T) {
	es := echo.NewServer()
	defer es.Close()

	cliConf, _, err := GenerateConfig("127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	dialer, err := newStreamDialer(cliConf.Proxy)
	if err != nil {
		t.Fatal(err)
	}
	dialer = bypassPrivate(dialer)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	stream, err := dialer.Dial(ctx, es.Listener.Addr().String())
	if err != nil {
		// If dialer does not connect to localServer directly, Dial fails
		// because no server for transport is running
		t.Fatalf("got error: %s, want nil", err)
	}
	stream.Close()
}

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

var (
	httpListen  string
	socksListen string
)

func runTestServer() (clients, servers []any, listenHTTP, listenSOCKS string) {
	cliConf, srvConf, err := GenerateConfig("127.0.0.1")
	if err != nil {
		panic(err)
	}

	srvClosers, err := StartServices(srvConf)
	if err != nil {
		panic(err)
	}

	cliConf.Proxy.allowPrivate = true
	cliClosers, err := StartServices(cliConf)
	if err != nil {
		panic(err)
	}

	return cliClosers, srvClosers, cliConf.ListenHTTP, cliConf.ListenSOCKS
}

func TestHttpProxyToGRPC(t *testing.T) {
	clients, servers, listenHTTP, _ := runTestServer()
	defer closeAll(clients)
	defer closeAll(servers)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "1")
	}))
	defer ts.Close()

	pu, err := url.Parse("http://" + listenHTTP)
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
	cliConf, srvConf, err := GenerateConfig("127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	cliConf.Proxy.Proto = ProtoHTTP2
	srvConf.Listen[0].Proto = ProtoHTTP2
	srvClosers, err := StartServices(srvConf)
	if err != nil {
		t.Fatal(err)
	}
	cliConf.Proxy.allowPrivate = true
	cliClosers, err := StartServices(cliConf)
	if err != nil {
		t.Fatal(err)
	}
	defer closeAll(cliClosers)
	defer closeAll(srvClosers)

	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "1")
	}))
	defer ts.Close()

	pu, err := url.Parse("http://" + cliConf.ListenHTTP)
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
	clients, servers, _, listenSOCKS := runTestServer()
	defer closeAll(clients)
	defer closeAll(servers)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "1")
	}))
	defer ts.Close()

	pu, err := url.Parse("socks5://" + listenSOCKS)
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

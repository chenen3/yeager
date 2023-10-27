package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

var (
	httpListen  string
	socksListen string
)

func runTestServer() (clients, servers []io.Closer, listenHTTP, listenSOCKS string) {
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

func TestHttpProxyToTunnel(t *testing.T) {
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

func TestSocksProxyToTunnel(t *testing.T) {
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

func TestPrivate(t *testing.T) {
	cliConf, _, err := GenerateConfig("127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	// private address is not allowed by default
	d, err := newProxyDialer(cliConf.Proxy)
	if err != nil {
		t.Fatal(err)
	}
	hosts := []string{"localhost", "127.0.0.1", "192.168.1.1"}
	for i := range hosts {
		rwc, err := d.Dial(context.Background(), "tcp", hosts[i])
		if err == nil {
			defer rwc.Close()
			t.Fatal("expected error")
		}
	}
}

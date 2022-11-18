package main

import (
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

func TestMain(m *testing.M) {
	srvConf, cliConf, err := makeConfig("127.0.0.1")
	if err != nil {
		panic(err)
	}
	srvClosers, err := StartServices(srvConf)
	defer CloseAll(srvClosers)
	if err != nil {
		panic(err)
	}
	cliClosers, err := StartServices(cliConf)
	defer CloseAll(cliClosers)
	if err != nil {
		panic(err)
	}

	httpListen = cliConf.HTTPListen
	socksListen = cliConf.SOCKSListen
	m.Run()
}

func TestHttpProxyToTunnel(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "1")
	}))
	defer ts.Close()

	pu, err := url.Parse("http://" + httpListen)
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
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "1")
	}))
	defer ts.Close()

	pu, err := url.Parse("socks5://" + socksListen)
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

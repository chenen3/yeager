package main

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/chenen3/yeager/config"
)

func init() {
	replace := func(groups []string, a slog.Attr) slog.Attr {
		// the time key is unnecessary for testing
		if a.Key == slog.TimeKey && len(groups) == 0 {
			return slog.Attr{}
		}
		// Remove the directory from the source's filename.
		if a.Key == slog.SourceKey {
			source := a.Value.Any().(*slog.Source)
			source.File = filepath.Base(source.File)
		}
		return a
	}
	debugLogger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		AddSource:   true,
		Level:       slog.LevelDebug,
		ReplaceAttr: replace,
	}))
	slog.SetDefault(debugLogger)
}

var (
	httpListen  string
	socksListen string
)

func TestMain(m *testing.M) {
	cliConf, srvConf, err := config.Generate("127.0.0.1")
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

	httpListen = cliConf.ListenHTTP
	socksListen = cliConf.ListenSOCKS
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

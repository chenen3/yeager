package main

import (
	"crypto/tls"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/chenen3/yeager/config"
)

func TestProxyToGRPC(t *testing.T) {
	cc, sc, err := config.Generate("127.0.0.1")
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

	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "1")
	}))
	defer ts.Close()

	t.Run("https2grpc", func(t *testing.T) {
		pu, err := url.Parse("http://" + cc.HTTPProxy)
		if err != nil {
			t.Error(err)
			return
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
			t.Error(err)
			return
		}
		defer resp.Body.Close()
		bs, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Error(err)
			return
		}
		if string(bs) != "1" {
			t.Errorf("want 1, got %s", bs)
			return
		}
	})

	// how it works: client request -> socks proxy -> grpc client -> grpc server -> http test server
	t.Run("socks2grpc", func(t *testing.T) {
		pu, err := url.Parse("socks5://" + cc.SOCKSProxy)
		if err != nil {
			t.Error(err)
			return
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
			t.Error(err)
			return
		}
		defer resp.Body.Close()
		bs, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Error(err)
			return
		}
		if string(bs) != "1" {
			t.Errorf("want 1, got %s", bs)
			return
		}
	})
}

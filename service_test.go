package main

import (
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/chenen3/yeager/config"
	"github.com/chenen3/yeager/proxy"
	"github.com/chenen3/yeager/transport"
	"github.com/chenen3/yeager/transport/https"
)

func localAddr() string {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	ln.Close()
	return ln.Addr().String()
}

func TestProxyToGRPC(t *testing.T) {
	cc, sc, err := config.Generate("127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}

	httpProxyAddr := localAddr()
	socks5ProxyAddr := localAddr()
	grpcAddr := localAddr()
	for i := range cc.Listen {
		switch cc.Listen[i].Protocol {
		case config.ProtoHTTP:
			cc.Listen[i].Address = httpProxyAddr
		case config.ProtoSOCKS5:
			cc.Listen[i].Address = socks5ProxyAddr
		}
	}
	for i := range cc.Transport {
		if cc.Transport[i].Protocol == config.ProtoGRPC {
			cc.Transport[i].Address = grpcAddr
		}
	}

	for i := range sc.Listen {
		if sc.Listen[i].Protocol == config.ProtoGRPC {
			sc.Listen[i].Address = grpcAddr
		}
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
		client := http.Client{
			Transport: &http.Transport{
				Proxy:           http.ProxyURL(&url.URL{Scheme: "http", Host: httpProxyAddr}),
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
		client := http.Client{
			Transport: &http.Transport{
				Proxy:           http.ProxyURL(&url.URL{Scheme: "socks5", Host: socks5ProxyAddr}),
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

func TestHTTPTransport(t *testing.T) {
	// client request -> [http proxy server A -> http transport] -> http proxy server B -> http test server
	hostport := localAddr()
	proxySrvA := &http.Server{Addr: localAddr(), Handler: proxy.NewHTTPHandler(&https.StreamDialer{HostPort: hostport})}
	go proxySrvA.ListenAndServe()
	defer proxySrvA.Close()

	proxySrvB := &http.Server{Addr: hostport, Handler: proxy.NewHTTPHandler(transport.TCPStreamDialer{})}
	go proxySrvB.ListenAndServe()
	defer proxySrvB.Close()

	testSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "1")
	}))
	defer testSrv.Close()

	client := http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyURL(&url.URL{Scheme: "http", Host: proxySrvA.Addr}),
		},
		Timeout: time.Second,
	}

	// the proxy services may not started yet
	time.Sleep(time.Millisecond)

	resp, err := client.Get(testSrv.URL)
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

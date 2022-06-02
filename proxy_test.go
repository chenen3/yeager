package main

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/chenen3/yeager/config"
	"github.com/chenen3/yeager/util"
)

var httpProxyURL string

func TestMain(m *testing.M) {
	cert, err := util.GenerateCertificate("127.0.0.1", false)
	if err != nil {
		panic(err)
	}

	tunnelPort, err := util.ChoosePort()
	if err != nil {
		panic(err)
	}
	srvConf := config.Config{
		Inbounds: config.Inbounds{
			Yeager: &config.YeagerServer{
				Listen:    fmt.Sprintf("127.0.0.1:%d", tunnelPort),
				Transport: config.TransGRPC,
				MutualTLS: config.MutualTLS{
					CertPEM: string(cert.ServerCert),
					KeyPEM:  string(cert.ServerKey),
					CAPEM:   string(cert.RootCert),
				},
			},
		},
	}
	serverProxy, err := NewProxy(srvConf)
	if err != nil {
		panic(err)
	}
	go serverProxy.Serve()
	defer serverProxy.Close()

	httpProxyPort, err := util.ChoosePort()
	if err != nil {
		panic(err)
	}
	httpProxyURL = fmt.Sprintf("http://127.0.0.1:%d", httpProxyPort)

	cliConf := config.Config{
		Inbounds: config.Inbounds{
			HTTP: &config.HTTP{
				Listen: fmt.Sprintf("127.0.0.1:%d", httpProxyPort),
			},
		},
		Outbounds: []config.YeagerClient{
			{
				Tag:       "PROXY",
				Address:   fmt.Sprintf("127.0.0.1:%d", tunnelPort),
				Transport: config.TransGRPC,
				MutualTLS: config.MutualTLS{
					CertPEM: string(cert.ClientCert),
					KeyPEM:  string(cert.ClientKey),
					CAPEM:   string(cert.RootCert),
				},
			},
		},
		Rules: []string{
			"FINAL,PROXY",
		},
	}
	clientProxy, err := NewProxy(cliConf)
	if err != nil {
		panic(err)
	}
	go clientProxy.Serve()
	defer clientProxy.Close()

	os.Exit(m.Run())
}

func TestProxy(t *testing.T) {
	// wait for the proxy server to start in the background
	time.Sleep(time.Millisecond)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "1")
	}))
	defer ts.Close()

	pu, err := url.Parse(httpProxyURL)
	if err != nil {
		t.Fatal(err)
	}
	client := http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyURL(pu),
		},
		Timeout: time.Second,
	}
	// traffic direction: client request -> http proxy -> tunnel client -> tunnel server -> http test server
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

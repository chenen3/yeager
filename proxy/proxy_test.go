package proxy

import (
	"encoding/json"
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

	// setup proxy server
	yeagerProxyPort, err := util.ChoosePort()
	if err != nil {
		panic(err)
	}
	srvConf, err := makeServerProxyConf(yeagerProxyPort, cert)
	if err != nil {
		panic(err)
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

	cliConf, err := makeClientProxyConf(httpProxyPort, yeagerProxyPort, cert)
	if err != nil {
		panic(err)
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
		_, err := io.WriteString(w, "1")
		if err != nil {
			t.Error(err)
			return
		}
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
	// traffic direction: client request -> inbound http proxy -> outbound yeager proxy -> inbound yeager proxy -> http test server
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

func makeClientProxyConf(inboundPort, outboundPort int, cert *util.Cert) (*config.Config, error) {
	s := fmt.Sprintf(`{
    "inbounds": {
		"http": {
            "listen": "127.0.0.1:%d"
        }
	},
    "outbounds": [
        {
            "tag": "PROXY",
            "address": "127.0.0.1:%d",
            "transport": "tls"
        }
    ],
    "rules": [
        "FINAL,PROXY"
    ]
}`, inboundPort, outboundPort)
	conf, err := config.LoadJSON([]byte(s))
	if err != nil {
		return nil, err
	}

	conf.Outbounds[0].MutualTLS = config.MutualTLS{
		CertPEM: cert.ClientCert,
		KeyPEM:  cert.ClientKey,
		CAPEM:   cert.RootCert,
	}
	return conf, nil
}

func makeServerProxyConf(inboundPort int, cert *util.Cert) (*config.Config, error) {
	s := fmt.Sprintf(`{
    "inbounds": {
        "yeager": {
            "listen": "127.0.0.1:%d",
            "transport": "tls"
        }
    }
}`, inboundPort)

	conf := new(config.Config)
	if err := json.Unmarshal([]byte(s), conf); err != nil {
		return nil, err
	}

	conf.Inbounds.Yeager.MutualTLS = config.MutualTLS{
		CertPEM: cert.ServerCert,
		KeyPEM:  cert.ServerKey,
		CAPEM:   cert.RootCert,
	}
	return conf, nil
}

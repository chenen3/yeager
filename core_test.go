package yeager

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

	"yeager/config"
	"yeager/util"
)

var httpProxyURL string

func TestMain(m *testing.M) {
	// setup proxy server
	httpProxyPort, err := util.ChoosePort()
	if err != nil {
		return
	}
	httpProxyURL = fmt.Sprintf("http://127.0.0.1:%d", httpProxyPort)

	yeagerProxyPort, err := util.ChoosePort()
	if err != nil {
		return
	}
	cliConf, err := makeClientProxyConf(httpProxyPort, yeagerProxyPort)
	if err != nil {
		return
	}
	clientProxy, err := NewProxy(cliConf)
	if err != nil {
		return
	}
	clientProxy.Start()
	defer clientProxy.Close()

	srvConf, err := makeServerProxyConf(yeagerProxyPort)
	if err != nil {
		return
	}
	serverProxy, err := NewProxy(srvConf)
	if err != nil {
		return
	}
	serverProxy.Start()
	defer serverProxy.Close()

	code := m.Run()
	os.Exit(code)
}

func TestCore(t *testing.T) {
	// wait for the proxy server to start in the background
	time.Sleep(time.Millisecond)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "1")
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
	// flow direction: client request -> inbound http proxy -> outbound yeager proxy -> inbound yeager proxy -> http test server
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

func makeClientProxyConf(inboundPort, outboundPort int) (*config.Config, error) {
	s := fmt.Sprintf(`{
    "inbounds": {
		"http": {
            "address": "127.0.0.1:%d"
        }
	},
    "outbounds": [
        {
            "tag": "PROXY",
            "address": "127.0.0.1:%d",
            "uuid": "51aef373-e1f7-4257-a45d-e75e65d712c4",
            "transport": "tls",
			"insecure": true
        }
    ],
    "rules": [
        "FINAL,PROXY"
    ]
}`, inboundPort, outboundPort)
	return config.LoadJSON([]byte(s))
}

func makeServerProxyConf(inboundPort int) (*config.Config, error) {
	s := fmt.Sprintf(`{
    "inbounds": {
        "yeager": {
            "address": "127.0.0.1:%d",
            "uuid": "51aef373-e1f7-4257-a45d-e75e65d712c4",
            "transport": "tls"
        }
    }
}`, inboundPort)

	conf := new(config.Config)
	if err := json.Unmarshal([]byte(s), conf); err != nil {
		return nil, err
	}

	certPEM, keyPEM, err := util.SelfSignedCertificate()
	if err != nil {
		return nil, err
	}
	conf.Inbounds.Yeager.CertPEMBlock = certPEM
	conf.Inbounds.Yeager.KeyPEMBlock = keyPEM
	return conf, nil
}

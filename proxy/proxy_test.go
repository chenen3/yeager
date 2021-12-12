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

var (
	httpProxyURL   string
	socks5ProxyURL string
)

func TestMain(m *testing.M) {
	// setup proxy server
	yeagerProxyPort, err := util.ChoosePort()
	if err != nil {
		panic(err)
	}
	srvConf, err := makeServerProxyConf(yeagerProxyPort)
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
	socks5ProxyPort, err := util.ChoosePort()
	if err != nil {
		panic(err)
	}
	socks5ProxyURL = fmt.Sprintf("socks5://127.0.0.1:%d", socks5ProxyPort)

	cliConf, err := makeClientProxyConf(httpProxyPort, socks5ProxyPort, yeagerProxyPort)
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

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "1")
	}))
	defer ts.Close()

	var tests = []struct {
		name            string
		inboundProxyUrl string
	}{
		{
			name: "httpProxy", inboundProxyUrl: httpProxyURL,
		},
		{
			name: "socks5Proxy", inboundProxyUrl: socks5ProxyURL,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			pu, err := url.Parse(test.inboundProxyUrl)
			if err != nil {
				t.Fatal(err)
			}
			client := http.Client{
				Transport: &http.Transport{
					Proxy: http.ProxyURL(pu),
				},
				Timeout: time.Second,
			}
			// traffic direction: client request -> inbound proxy -> outbound yeager proxy -> inbound yeager proxy -> http test server
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

		})
	}
}

func makeClientProxyConf(httpProxyPort, socksProxyPort, outboundPort int) (*config.Config, error) {
	s := fmt.Sprintf(`{
    "inbounds": {
		"http": {
            "listen": "127.0.0.1:%d"
        },
		"socks": {
			"listen": "127.0.0.1:%d"
		}
	},
    "outbounds": [
        {
            "tag": "PROXY",
            "address": "127.0.0.1:%d",
            "uuid": "51aef373-e1f7-4257-a45d-e75e65d712c4",
            "transport": "tcp",
			"security": "tls",
			"tls": {
				"insecure": true
			}
        }
    ],
    "rules": [
        "FINAL,PROXY"
    ]
}`, httpProxyPort, socksProxyPort, outboundPort)
	return config.LoadJSON([]byte(s))
}

func makeServerProxyConf(inboundPort int) (*config.Config, error) {
	s := fmt.Sprintf(`{
    "inbounds": {
        "yeager": {
            "listen": "127.0.0.1:%d",
            "uuid": "51aef373-e1f7-4257-a45d-e75e65d712c4",
            "transport": "tcp",
			"security": "tls"
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
	conf.Inbounds.Yeager.TLS.CertPEM = certPEM
	conf.Inbounds.Yeager.TLS.KeyPEM = keyPEM
	return conf, nil
}

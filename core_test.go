package yeager

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path"
	"testing"
	"time"
	"yeager/config"
	"yeager/util"
)

func TestCore(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	proxyUrl, err := setupHttp2YeagerProxy(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// wait for the proxy server to start in the background
	time.Sleep(time.Millisecond)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "1")
	}))
	defer ts.Close()

	pu, err := url.Parse(proxyUrl)
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

func setupHttp2YeagerProxy(ctx context.Context) (proxyUrl string, err error) {
	httpProxyPort, err := util.ChoosePort()
	if err != nil {
		return
	}
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
	go clientProxy.Start(ctx)

	srvConf, err := makeServerProxyConf(yeagerProxyPort)
	if err != nil {
		return
	}
	serverProxy, err := NewProxy(srvConf)
	if err != nil {
		return
	}
	go serverProxy.Start(ctx)

	return fmt.Sprintf("http://127.0.0.1:%d", httpProxyPort), nil
}

func makeClientProxyConf(inboundPort, outboundPort int) (config.Config, error) {
	var clientConf config.Config
	s := fmt.Sprintf(`{
    "inbounds": [
        {
            "protocol": "http",
            "setting": {
				"host": "127.0.0.1",
                "port": %d
            }
        }
    ],
    "outbounds": [
        {
            "tag": "PROXY",
            "protocol": "armin",
            "setting": {
				"host": "127.0.0.1",
                "port": %d,
                "uuid": "51aef373-e1f7-4257-a45d-e75e65d712c4",
				"transport": "tls",
				"tls": {
					"insecure": true
				}
            }
        }
    ],
    "rules": [
        "FINAL,PROXY"
    ]
}`, inboundPort, outboundPort)
	err := json.Unmarshal([]byte(s), &clientConf)
	return clientConf, err
}

func makeServerProxyConf(inboundPort int) (config.Config, error) {
	var conf config.Config
	dir, err := os.Getwd()
	if err != nil {
		return config.Config{}, err
	}
	certFile := path.Join(dir, "config", "dev", "cert.pem")
	keyFile := path.Join(dir, "config", "dev", "key.pem")
	s := fmt.Sprintf(`{
    "inbounds": [
        {
            "protocol": "armin",
            "setting": {
				"host": "127.0.0.1",
                "port": %d,
                "uuid": "51aef373-e1f7-4257-a45d-e75e65d712c4",
				"transport": "tls",
				"tls": {
					"certFile": "%s",
					"keyFile": "%s"
				}
            }
        }
    ]
}`, inboundPort, certFile, keyFile)
	err = json.Unmarshal([]byte(s), &conf)
	return conf, err
}

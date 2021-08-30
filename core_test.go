package yeager

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"

	"yeager/config"
	"yeager/util"
)

var certFilename, keyFilename string

func TestMain(m *testing.M) {
	key, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		panic(err)
	}
	template := x509.Certificate{SerialNumber: big.NewInt(1)}
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		panic(err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	keyFile, err := os.CreateTemp("", "pem")
	if err != nil {
		panic(err)
	}
	defer keyFile.Close()
	_, err = keyFile.Write(keyPEM)
	if err != nil {
		panic(err)
	}
	keyFilename = keyFile.Name()

	certFile, err := os.CreateTemp("", "pem")
	if err != nil {
		panic(err)
	}
	defer certFile.Close()
	_, err = certFile.Write(certPEM)
	if err != nil {
		panic(err)
	}
	certFilename = certFile.Name()

	code := m.Run()
	os.Exit(code)
}

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
	return config.LoadBytes([]byte(s))
}

func makeServerProxyConf(inboundPort int) (*config.Config, error) {
	s := fmt.Sprintf(`{
    "inbounds": {
        "armin": {
            "address": "127.0.0.1:%d",
            "uuid": "51aef373-e1f7-4257-a45d-e75e65d712c4",
            "transport": "tls",
			"certFile": "%s",
			"keyFile": "%s"
        }
    }
}`, inboundPort, certFilename, keyFilename)
	return config.LoadBytes([]byte(s))
}

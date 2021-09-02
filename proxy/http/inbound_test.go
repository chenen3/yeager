package http

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
	"yeager/config"
	"yeager/proxy"

	"yeager/log"
	"yeager/util"
)

func TestServer(t *testing.T) {
	port, err := util.ChoosePort()
	if err != nil {
		t.Fatal(err)
	}

	server := NewServer(&config.HTTPServerConfig{
		Address: fmt.Sprintf("127.0.0.1:%d", port),
	})
	defer server.Close()
	go func() {
		t.Log(server.ListenAndServe(func(ctx context.Context, conn net.Conn, addr *proxy.Address) {
			defer conn.Close()
			if addr.String() != "fake.domain.com:1234" {
				t.Errorf("received unexpected dst addr: %s", addr.String())
				return
			}

			rec := httptest.NewRecorder()
			_, err = rec.WriteString("1")
			if err != nil {
				t.Error(err)
				return
			}

			err = rec.Result().Write(conn)
			if err != nil {
				t.Error(err)
				return
			}
		}))
	}()

	<-server.ready
	proxyUrl, _ := url.Parse("http://" + server.conf.Address)
	client := &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyURL(proxyUrl),
			DialContext: (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
		},
	}
	res, err := client.Get("http://fake.domain.com:1234")
	if err != nil {
		log.Error(err)
		return
	}
	defer res.Body.Close()
	io.Copy(io.Discard, res.Body)
}

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

	"github.com/chenen3/yeager/config"
	"github.com/chenen3/yeager/util"
	"go.uber.org/zap"
)

func TestServer(t *testing.T) {
	port, err := util.ChoosePort()
	if err != nil {
		t.Fatal(err)
	}

	server, err := NewServer(&config.HTTPProxy{
		Listen: fmt.Sprintf("127.0.0.1:%d", port),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	go func() {
		err := server.ListenAndServe(func(ctx context.Context, conn net.Conn, addr string) {
			defer conn.Close()
			if addr != "fake.domain.com:1234" {
				t.Errorf("received unexpected dst addr: %s", addr)
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
		})
		if err != nil {
			t.Error(err)
		}
	}()

	<-server.ready
	proxyUrl, _ := url.Parse("http://" + server.conf.Listen)
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
		zap.S().Error(err)
		return
	}
	defer res.Body.Close()
	io.Copy(io.Discard, res.Body)
}

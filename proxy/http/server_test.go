package http

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/chenen3/yeager/log"
	"github.com/chenen3/yeager/util"
)

func TestServer(t *testing.T) {
	port, err := util.ChoosePort()
	if err != nil {
		t.Fatal(err)
	}

	srv, err := NewServer(fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	srv.Handle(func(conn net.Conn, addr string) {
		defer conn.Close()
		if addr != "fake.domain.com:1234" {
			panic("received unexpected dst addr: " + addr)
		}

		rec := httptest.NewRecorder()
		_, e := rec.WriteString("1")
		if e != nil {
			log.L().Error(e)
			return
		}

		e = rec.Result().Write(conn)
		if e != nil {
			log.L().Error(e)
			return
		}
	})
	go func() {
		err = srv.ListenAndServe()
		if err != nil {
			log.L().Error(err)
		}
	}()

	select {
	case <-time.After(time.Second):
		t.Fatal("server not ready in time")
	case <-srv.ready:
	}

	proxyUrl, _ := url.Parse("http://" + srv.addr)
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
		log.L().Error(err)
		return
	}
	defer res.Body.Close()
	bs, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(bs) != "1" {
		t.Fatalf("want 1, got %s", bs)
	}
}

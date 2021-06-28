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

	"yeager/log"
	"yeager/util"
)

func TestServer(t *testing.T) {
	port, err := util.ChoosePort()
	if err != nil {
		t.Fatal(err)
	}
	ps := NewServer(&Config{
		Host: "127.0.0.1",
		Port: port,
	})
	go ps.Serve()
	defer ps.Close()
	// wait for the proxy server to start in the background
	time.Sleep(time.Millisecond)

	go func() {
		proxyUrl, _ := url.Parse(fmt.Sprintf("http://%s:%d", ps.conf.Host, ps.conf.Port))
		client := &http.Client{
			Transport: &http.Transport{
				Proxy: http.ProxyURL(proxyUrl),
				DialContext: (&net.Dialer{
					Timeout:   30 * time.Second,
					KeepAlive: 30 * time.Second,
				}).DialContext,
			},
		}
		res, err := client.Get("http://1.2.3.4")
		if err != nil {
			log.Error(err)
			return
		}
		defer res.Body.Close()
		io.Copy(io.Discard, res.Body)
	}()

	conn := <-ps.Accept()
	defer conn.Close()
	if got := conn.DstAddr().Host; got != "1.2.3.4" {
		t.Fatalf("proxy server got wrong destination address: %s", got)
	}
	rec := httptest.NewRecorder()
	_, err = rec.WriteString("1")
	if err != nil {
		t.Fatal(err)
	}
	err = rec.Result().Write(conn)
	if err != nil {
		t.Fatal(err)
	}
}

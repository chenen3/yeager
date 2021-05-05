package http

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"yeager/common"
)

func TestServer(t *testing.T) {
	port, err := common.ChoosePort()
	if err != nil {
		t.Fatal(err)
	}
	ps := NewServer(&Config{
		Host: "127.0.0.1",
		Port: port,
	})
	go ps.Serve()
	defer ps.Close()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "1")
	}))
	defer ts.Close()

	err = os.Setenv("HTTP_PROXY", fmt.Sprintf("http://%s:%d", ps.conf.Host, ps.conf.Port))
	if err != nil {
		t.Fatal(err)
	}
	res, err := http.Get(ts.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := io.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if string(resp) != "1" {
		t.Fatalf("want 1, got %s", resp)
	}
}

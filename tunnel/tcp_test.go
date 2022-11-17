package tunnel

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/chenen3/yeager/util"
)

var tcptun TcpTunnel

func TestMain(m *testing.M) {
	port, _ := util.AllocatePort()
	ready := make(chan struct{})
	tcptun = TcpTunnel{Address: fmt.Sprintf("127.0.0.1:%d", port)}
	defer tcptun.Close()
	go func() {
		close(ready)
		err := tcptun.Serve()
		if err != nil {
			panic(err)
		}
	}()
	m.Run()
}

func TestTcpTunnel(t *testing.T) {
	want := "ok"
	hs := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, want)
	}))
	defer hs.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	// the tunnel server may not started yet
	time.Sleep(time.Millisecond)
	rwc, err := tcptun.DialContext(ctx, hs.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer rwc.Close()

	req, err := http.NewRequest("GET", hs.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	err = req.Write(rwc)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.ReadResponse(bufio.NewReader(rwc), req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	got, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("want %s, got %s", want, got)
	}
}

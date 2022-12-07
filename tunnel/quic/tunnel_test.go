package quic

import (
	"bufio"
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/chenen3/yeager/cert"
)

func TestQuicTunnel(t *testing.T) {
	want := "ok"
	hs := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, want)
	}))
	defer hs.Close()

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	lis.Close()

	ct, err := cert.Generate("127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	srvTLSConf, err := cert.MakeServerTLSConfig(ct.RootCert, ct.ServerCert, ct.ServerKey)
	if err != nil {
		t.Fatal(err)
	}

	var ts TunnelServer
	defer ts.Close()
	ready := make(chan struct{})
	go func() {
		close(ready)
		if e := ts.Serve(lis.Addr().String(), srvTLSConf); e != nil {
			t.Error(e)
		}
	}()

	cliTLSConf, err := cert.MakeClientTLSConfig(ct.RootCert, ct.ClientCert, ct.ClientKey)
	if err != nil {
		t.Fatal(err)
	}
	tc := NewTunnelClient(lis.Addr().String(), cliTLSConf, 1)
	defer tc.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	<-ready
	rwc, err := tc.DialContext(ctx, hs.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}

	req, err := http.NewRequest("GET", hs.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = req.Write(rwc); err != nil {
		t.Fatal(err)
	}
	resp, err := http.ReadResponse(bufio.NewReader(rwc), req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("bad status: %s", resp.Status)
	}
	got, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("want %s, got %s", want, got)
	}
}

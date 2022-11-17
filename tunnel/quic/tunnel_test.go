package quic

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/chenen3/yeager/util"
)

func generateTLSConfig() *tls.Config {
	cert, key, err := util.SelfSignedCertificate()
	if err != nil {
		panic(err)
	}
	tlsCert, err := tls.X509KeyPair(cert, key)
	if err != nil {
		panic(err)
	}
	return &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
	}
}

func TestQuicTunnel(t *testing.T) {
	want := "ok"
	hs := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, want)
	}))
	defer hs.Close()

	port, _ := util.AllocatePort()
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	tlsConf := generateTLSConfig()
	ready := make(chan struct{})

	var ts TunnelServer
	defer ts.Close()
	go func() {
		close(ready)
		err := ts.Serve(addr, tlsConf)
		if err != nil {
			t.Error(err)
		}
	}()

	tc := NewTunnelClient(addr, &tls.Config{InsecureSkipVerify: true}, 1)
	defer tc.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	rwc, err := tc.DialContext(ctx, hs.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}

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

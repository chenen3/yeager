package quic

import (
	"bufio"
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/chenen3/yeager/cert"
)

func generateTLSConfig() *tls.Config {
	cert, key, err := cert.SelfSign()
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

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	lis.Close()

	cert, key, err := cert.SelfSign()
	if err != nil {
		t.Fatal(err)
	}
	tlsCert, err := tls.X509KeyPair(cert, key)
	if err != nil {
		t.Fatal(err)
	}
	tlsConf := &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
	}

	var ts TunnelServer
	defer ts.Close()
	ready := make(chan struct{})
	go func() {
		close(ready)
		err := ts.Serve(lis.Addr().String(), tlsConf)
		if err != nil {
			t.Error(err)
		}
	}()

	<-ready
	tc := NewTunnelClient(lis.Addr().String(), &tls.Config{InsecureSkipVerify: true}, 1)
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

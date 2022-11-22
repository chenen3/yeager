package grpc

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/chenen3/yeager/util"
)

func generateTLSConfig() *tls.Config {
	certPEM, keyPEM, err := util.SelfSignedCertificate()
	if err != nil {
		panic(err)
	}
	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		panic(err)
	}
	return &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
	}
}

func TestGrpcTunnel(t *testing.T) {
	ok := "ok"
	hs := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, ok)
	}))
	defer hs.Close()

	port, _ := util.AllocatePort()
	address := fmt.Sprintf("127.0.0.1:%d", port)
	ready := make(chan struct{})

	ts := new(TunnelServer)
	defer ts.Close()
	go func() {
		tlsConf := generateTLSConfig()
		close(ready)
		err := ts.Serve(address, tlsConf)
		if err != nil && !errors.Is(err, net.ErrClosed) {
			t.Error(err)
		}
	}()

	tc := NewTunnelClient(address, &tls.Config{InsecureSkipVerify: true}, 1)
	defer tc.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	<-ready
	// the proxy server may not started yet
	time.Sleep(time.Millisecond)
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
	if string(got) != ok {
		t.Fatalf("want %s, got %s", ok, got)
	}
}

func TestDial_Parallel(t *testing.T) {
	ok := "ok"
	hs := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, ok)
	}))
	defer hs.Close()

	port, _ := util.AllocatePort()
	address := fmt.Sprintf("127.0.0.1:%d", port)
	ready := make(chan struct{})

	ts := new(TunnelServer)
	defer ts.Close()
	go func() {
		tlsConf := generateTLSConfig()
		close(ready)
		err := ts.Serve(address, tlsConf)
		if err != nil && !errors.Is(err, net.ErrClosed) {
			t.Error(err)
		}
	}()

	tc := NewTunnelClient(address, &tls.Config{InsecureSkipVerify: true}, 1)
	defer tc.Close()
	<-ready
	// the proxy server may not started yet
	time.Sleep(time.Millisecond)
	t.Run("group", func(t *testing.T) {
		parallelTest := func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			rwc, err := tc.DialContext(ctx, hs.Listener.Addr().String())
			if err != nil {
				t.Error(err)
				return
			}
			req, err := http.NewRequest("GET", hs.URL, nil)
			if err != nil {
				t.Error(err)
				return
			}
			if err = req.Write(rwc); err != nil {
				t.Error(err)
				return
			}
			resp, err := http.ReadResponse(bufio.NewReader(rwc), req)
			if err != nil {
				t.Error(err)
				return
			}
			defer resp.Body.Close()
			if resp.StatusCode != 200 {
				t.Errorf("bad status: %s", resp.Status)
			}
			got, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Error(err)
				return
			}
			if string(got) != ok {
				t.Errorf("want %s, got %s", ok, got)
				return
			}
		}
		t.Run("test1", parallelTest)
		t.Run("test2", parallelTest)
		t.Run("test3", parallelTest)
	})
}
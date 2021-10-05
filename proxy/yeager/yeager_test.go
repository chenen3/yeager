package yeager

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"testing"
	"time"

	"github.com/chenen3/yeager/config"
	"github.com/chenen3/yeager/util"
)

var keyPEM, certPEM []byte

func TestMain(m *testing.M) {
	var err error
	certPEM, keyPEM, err = util.SelfSignedCertificate()
	if err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}

func serveTLS() (*Server, error) {
	port, err := util.ChoosePort()
	if err != nil {
		return nil, err
	}
	return NewServer(&config.YeagerServer{
		Listen:    fmt.Sprintf("127.0.0.1:%d", port),
		UUID:      "ce9f7ded-027c-e7b3-9369-308b7208d498",
		Transport: config.TransTCP,
		Security:  config.TLS,
		TLS:       config.Tls{CertPEM: certPEM, KeyPEM: keyPEM},
	})
}

func TestYeager_tls(t *testing.T) {
	server, err := serveTLS()
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
			io.Copy(conn, conn)
		})
		if err != nil {
			t.Error(err)
		}
	}()

	// FIXME: if failed to launch server, here blocks forever
	<-server.ready
	client, err := NewClient(&config.YeagerClient{
		Address:   server.conf.Listen,
		UUID:      server.conf.UUID,
		Transport: config.TransTCP,
		Security:  config.ClientTLS,
		TLS:       config.ClientTls{Insecure: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	ctx2, cancel2 := context.WithTimeout(context.Background(), time.Second)
	defer cancel2()
	conn, err := client.DialContext(ctx2, "fake.domain.com:1234")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	want := []byte("1")
	_, err = conn.Write(want)
	if err != nil {
		t.Fatal(err)
	}
	got := make([]byte, 1)
	io.ReadFull(conn, got)
	if !bytes.Equal(want, got) {
		t.Fatalf("want %v, got %v", want, got)
	}
}

func serveGRPC() (*Server, error) {
	port, err := util.ChoosePort()
	if err != nil {
		return nil, err
	}
	return NewServer(&config.YeagerServer{
		Listen:    fmt.Sprintf("127.0.0.1:%d", port),
		UUID:      "ce9f7ded-027c-e7b3-9369-308b7208d498",
		Transport: config.TransGRPC,
		Security:  config.TLS,
		TLS: config.Tls{
			CertPEM: certPEM,
			KeyPEM:  keyPEM,
		},
	})
}

func TestYeager_grpc(t *testing.T) {
	server, err := serveGRPC()
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
			io.Copy(conn, conn)
		})
		if err != nil {
			t.Error(err)
		}
	}()

	<-server.ready
	client, err := NewClient(&config.YeagerClient{
		Address:   server.conf.Listen,
		UUID:      server.conf.UUID,
		Transport: config.TransGRPC,
		Security:  config.ClientTLS,
		TLS:       config.ClientTls{Insecure: true},
	})
	if err != nil {
		t.Fatal("NewClient err: " + err.Error())
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	conn, err := client.DialContext(ctx, "fake.domain.com:1234")
	if err != nil {
		t.Fatal("dial err: " + err.Error())
	}
	defer conn.Close()

	want := []byte("1")
	_, err = conn.Write(want)
	if err != nil {
		t.Fatal(err)
	}
	got := make([]byte, 1)
	io.ReadFull(conn, got)
	if !bytes.Equal(want, got) {
		t.Fatalf("want %v, got %v", want, got)
	}
}

func TestYeager_mutualTLS(t *testing.T) {
	certInfo, err := util.GenerateCertificate("127.0.0.1", false)
	if err != nil {
		t.Fatal(err)
	}

	port, err := util.ChoosePort()
	if err != nil {
		t.Fatal(err)
	}
	server, err := NewServer(&config.YeagerServer{
		Listen:    fmt.Sprintf("127.0.0.1:%d", port),
		Transport: config.TransTCP,
		Security:  config.TLSMutual,
		MTLS: config.Mtls{
			CertPEM:  certInfo.ServerCert,
			KeyPEM:   certInfo.ServerKey,
			ClientCA: certInfo.RootCert,
		},
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
			io.Copy(conn, conn)
		})
		if err != nil {
			t.Error(err)
		}
	}()

	<-server.ready
	client, err := NewClient(&config.YeagerClient{
		Address:   server.conf.Listen,
		Transport: config.TransTCP,
		Security:  config.ClientTLSMutual,
		MTLS: config.ClientMTLS{
			CertPEM: certInfo.ClientCert,
			KeyPEM:  certInfo.ClientKey,
			RootCA:  certInfo.RootCert,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	ctx2, cancel2 := context.WithTimeout(context.Background(), time.Second)
	defer cancel2()
	conn, err := client.DialContext(ctx2, "fake.domain.com:1234")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	want := []byte("1")
	_, err = conn.Write(want)
	if err != nil {
		t.Fatal(err)
	}
	got := make([]byte, 1)
	io.ReadFull(conn, got)
	if !bytes.Equal(want, got) {
		t.Fatalf("want %v, got %v", want, got)
	}

}

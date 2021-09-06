package yeager

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"os"
	"testing"
	"time"

	"yeager/config"
	"yeager/util"
)

var keyPEM, certPEM []byte

func TestMain(m *testing.M) {
	key, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		panic(err)
	}
	template := x509.Certificate{SerialNumber: big.NewInt(1)}
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		panic(err)
	}
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	code := m.Run()
	os.Exit(code)
}

func serveTLS() (*Server, error) {
	port, err := util.ChoosePort()
	if err != nil {
		return nil, err
	}
	srv := NewServer(&config.YeagerServer{
		Address:      fmt.Sprintf("127.0.0.1:%d", port),
		UUID:         "ce9f7ded-027c-e7b3-9369-308b7208d498",
		Transport:    "tls",
		CertPEMBlock: certPEM,
		KeyPEMBlock:  keyPEM,
	})
	return srv, nil
}

func TestArmin_tls(t *testing.T) {
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

	<-server.ready
	client, err := NewClient(&config.YeagerClient{
		Address:   server.conf.Address,
		UUID:      server.conf.UUID,
		Transport: "tls",
		Insecure:  true,
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
	srv := NewServer(&config.YeagerServer{
		Address:      fmt.Sprintf("127.0.0.1:%d", port),
		UUID:         "ce9f7ded-027c-e7b3-9369-308b7208d498",
		Transport:    "grpc",
		CertPEMBlock: certPEM,
		KeyPEMBlock:  keyPEM,
	})
	return srv, nil
}

func TestArmin_grpc(t *testing.T) {
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
		Address:   server.conf.Address,
		UUID:      server.conf.UUID,
		Transport: "grpc",
		Insecure:  true,
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

func serveQUIC() (*Server, error) {
	port, err := util.ChoosePort()
	if err != nil {
		return nil, err
	}
	srv := NewServer(&config.YeagerServer{
		Address:      fmt.Sprintf("127.0.0.1:%d", port),
		UUID:         "ce9f7ded-027c-e7b3-9369-308b7208d498",
		Transport:    "quic",
		CertPEMBlock: certPEM,
		KeyPEMBlock:  keyPEM,
	})
	return srv, nil
}

func TestArmin_quic(t *testing.T) {
	server, err := serveQUIC()
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
		Address:   server.conf.Address,
		UUID:      server.conf.UUID,
		Transport: "quic",
		Insecure:  true,
	})
	if err != nil {
		t.Fatal("NewClient err: " + err.Error())
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
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

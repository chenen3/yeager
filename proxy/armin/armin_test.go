package armin

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"testing"
	"time"

	"yeager/proxy"
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
	return NewServer(&ServerConfig{
		Host:      "127.0.0.1",
		Port:      port,
		UUID:      "ce9f7ded-027c-e7b3-9369-308b7208d498",
		Transport: "tls",
		TLS: tlsServerConfig{
			certPEMBlock: certPEM,
			keyPEMBlock:  keyPEM,
		},
	})
}

func TestArmin_tls(t *testing.T) {
	server, err := serveTLS()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		t.Log(server.ListenAndServe(ctx))
	}()
	server.RegisterHandler(func(ctx context.Context, conn net.Conn, addr *proxy.Address) {
		defer conn.Close()
		if addr.String() != "fake.domain.com:1234" {
			t.Errorf("received unexpected dst addr: %s", addr.String())
			return
		}
		io.Copy(conn, conn)
	})

	<-server.ready
	client, err := NewClient(&ClientConfig{
		Host:      server.conf.Host,
		Port:      server.conf.Port,
		UUID:      server.conf.UUID,
		Transport: "tls",
		TLS: tlsClientConfig{
			ServerName: server.conf.Host,
			Insecure:   true,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	ctx2, cancel2 := context.WithTimeout(context.Background(), time.Second)
	defer cancel2()
	conn, err := client.DialContext(ctx2, proxy.NewAddress("fake.domain.com", 1234))
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
	return NewServer(&ServerConfig{
		Host:      "127.0.0.1",
		Port:      port,
		UUID:      "ce9f7ded-027c-e7b3-9369-308b7208d498",
		Transport: "grpc",
		TLS: tlsServerConfig{
			certPEMBlock: certPEM,
			keyPEMBlock:  keyPEM,
		},
	})
}

func TestArmin_grpc(t *testing.T) {
	server, err := serveGRPC()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		t.Log(server.ListenAndServe(ctx))
	}()
	server.RegisterHandler(func(ctx context.Context, conn net.Conn, addr *proxy.Address) {
		defer conn.Close()
		if addr.String() != "fake.domain.com:1234" {
			t.Errorf("received unexpected dst addr: %s", addr.String())
			return
		}
		io.Copy(conn, conn)
	})

	<-server.ready
	client, err := NewClient(&ClientConfig{
		Host:      server.conf.Host,
		Port:      server.conf.Port,
		UUID:      server.conf.UUID,
		Transport: "grpc",
		TLS: tlsClientConfig{
			ServerName: server.conf.Host,
			Insecure:   true,
		},
	})
	if err != nil {
		t.Fatal("NewClient err: " + err.Error())
	}
	defer client.Close()

	conn, err := client.DialContext(ctx, proxy.NewAddress("fake.domain.com", 1234))
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

var (
	fbHost = "127.0.0.1"
	fbPort = 5678
)

func serverWithFallback() (*Server, error) {
	port, err := util.ChoosePort()
	if err != nil {
		return nil, err
	}
	return NewServer(&ServerConfig{
		Host:      "127.0.0.1",
		Port:      port,
		UUID:      "ce9f7ded-027c-e7b3-9369-308b7208d498",
		Transport: "tls",
		TLS: tlsServerConfig{
			certPEMBlock: certPEM,
			keyPEMBlock:  keyPEM,
		},
		Fallback: fallback{
			Host: fbHost,
			Port: fbPort,
		},
	})
}

func TestFallback(t *testing.T) {
	server, err := serverWithFallback()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		t.Log(server.ListenAndServe(ctx))
	}()
	server.RegisterHandler(func(ctx context.Context, conn net.Conn, addr *proxy.Address) {
		defer conn.Close()
		want := fmt.Sprintf("%s:%d", fbHost, fbPort)
		if addr.String() != want {
			t.Errorf("received unexpected dst addr: %s", addr.String())
			return
		}
		io.Copy(conn, conn)
	})

	<-server.ready
	client := http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	}
	resp, err := client.Get(fmt.Sprintf("https://%s:%d", server.conf.Host, server.conf.Port))
	if err != nil {
		return
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
}

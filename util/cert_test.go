package util

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"testing"
	"time"
)

func TestGenerateCertificate(t *testing.T) {
	certInfo, err := GenerateCertificate("127.0.0.1", false)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	ok := pool.AppendCertsFromPEM(certInfo.RootCert)
	if !ok {
		t.Fatal("failed to parse root certificate")
	}

	cert, err := tls.X509KeyPair(certInfo.ServerCert, certInfo.ServerKey)
	if err != nil {
		t.Error(err)
		return
	}
	serverTLSConf := &tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    pool,
	}
	listener, err := tls.Listen("tcp", "127.0.0.1:", serverTLSConf)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	time.AfterFunc(time.Second, func() {
		listener.Close()
	})

	want := []byte{1}
	go func() {
		cert, err := tls.X509KeyPair(certInfo.ClientCert, certInfo.ClientKey)
		if err != nil {
			t.Error(err)
			return
		}
		tlsConf := &tls.Config{
			Certificates: []tls.Certificate{cert},
			RootCAs:      pool,
		}
		conn, err := tls.Dial("tcp", listener.Addr().String(), tlsConf)
		if err != nil {
			t.Log(listener.Addr().String())
			t.Error(err)
			return
		}
		defer conn.Close()
		_, err = conn.Write(want)
		if err != nil {
			t.Error(err)
			return
		}
	}()

	conn, err := listener.Accept()
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	got := make([]byte, len(want))
	_, err = conn.Read(got)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(want, got) {
		t.Fatalf("want %v, got %v", want, got)
	}
}

package config

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"testing"
	"time"
)

func TestNewCertificate(t *testing.T) {
	certInfo, err := newCert("127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	ok := pool.AppendCertsFromPEM(certInfo.rootCert)
	if !ok {
		t.Fatal("failed to parse root certificate")
	}

	cert, err := tls.X509KeyPair(certInfo.serverCert, certInfo.serverKey)
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
		cert, er := tls.X509KeyPair(certInfo.clientCert, certInfo.clientKey)
		if er != nil {
			t.Error(er)
			return
		}
		tlsConf := &tls.Config{
			Certificates: []tls.Certificate{cert},
			RootCAs:      pool,
		}
		conn, er := tls.Dial("tcp", listener.Addr().String(), tlsConf)
		if er != nil {
			t.Log(listener.Addr().String())
			t.Error(er)
			return
		}
		defer conn.Close()
		_, er = conn.Write(want)
		if er != nil {
			t.Error(er)
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

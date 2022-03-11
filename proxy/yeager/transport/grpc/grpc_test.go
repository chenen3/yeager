package grpc

import (
	"bytes"
	"context"
	"crypto/tls"
	"io"
	"testing"

	"github.com/chenen3/yeager/log"
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

func TestGRPC(t *testing.T) {
	lis, err := Listen("127.0.0.1:0", generateTLSConfig())
	if err != nil {
		t.Fatal(err)
	}
	defer lis.Close()

	go func() {
		conn, e := lis.Accept()
		if e != nil {
			log.Error(e)
			return
		}
		defer conn.Close()
		_, _ = io.Copy(conn, conn)
	}()

	d := NewDialer(&tls.Config{InsecureSkipVerify: true})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	conn, err := d.DialContext(ctx, lis.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	want := []byte{1}
	_, err = conn.Write(want)
	if err != nil {
		t.Fatal(err)
	}
	got := make([]byte, 1)
	_, err = conn.Read(got[:])
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("want %v, got %v", want, got)
	}
}

package quic

import (
	"bytes"
	"context"
	"crypto/tls"
	"io"
	"log"
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

func TestQUIC(t *testing.T) {
	lis, err := Listen("127.0.0.1:0", generateTLSConfig())
	if err != nil {
		t.Fatal(err)
	}
	defer lis.Close()

	go func() {
		serverConn, e := lis.Accept()
		if e != nil {
			log.Printf("quic listener accept err: %s", e)
			return
		}
		defer serverConn.Close()
		_, _ = io.Copy(serverConn, serverConn)
	}()

	cliTLSConf := &tls.Config{
		InsecureSkipVerify: true,
	}
	dialer := NewDialer(cliTLSConf, lis.Addr().String(), 1)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	clientConn, err := dialer.DialContext(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer clientConn.Close()

	ping := []byte("ping")
	_, err = clientConn.Write(ping)
	if err != nil {
		t.Fatal(err)
	}

	bs := make([]byte, len(ping))
	_, err = clientConn.Read(bs)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(bs, ping) {
		t.Fatalf("want ping, got %s", bs)
	}
}

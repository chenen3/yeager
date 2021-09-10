package quic

import (
	"bytes"
	"context"
	"crypto/tls"
	"io"
	"testing"
	"yeager/util"
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
		NextProtos:   []string{"quic-echo-example"},
	}
}

func TestQUIC(t *testing.T) {
	addr := "localhost:4242"
	lis, err := Listen(addr, generateTLSConfig())
	if err != nil {
		t.Fatal(err)
	}
	defer lis.Close()
	go func() {
		serverConn, err := lis.Accept()
		if err != nil {
			t.Error(err)
			return
		}
		defer serverConn.Close()
		io.Copy(serverConn, serverConn)
	}()

	cliTLSConf := &tls.Config{
		InsecureSkipVerify: true,
		NextProtos:         []string{"quic-echo-example"},
	}
	dialer := NewDialer(cliTLSConf)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	clientConn, err := dialer.DialContext(ctx, addr)
	defer clientConn.Close()
	if err != nil {
		t.Fatal(err)
	}

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
		t.Fatalf("want pong, got %s", bs)
	}
}

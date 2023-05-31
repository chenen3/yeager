package http2

import (
	"bytes"
	"context"
	"io"
	"log"
	"net"
	"testing"
	"time"

	"github.com/chenen3/yeager/cert"
	ynet "github.com/chenen3/yeager/net"
)

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}

func startTunnel() (*TunnelServer, *TunnelClient, error) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, nil, err
	}
	lis.Close()

	ct, err := cert.Generate("127.0.0.1")
	if err != nil {
		return nil, nil, err
	}
	srvTLSConf, err := cert.MakeServerTLSConfig(ct.RootCert, ct.ServerCert, ct.ServerKey)
	if err != nil {
		return nil, nil, err
	}

	ts := new(TunnelServer)
	go func() {
		if e := ts.Serve(lis.Addr().String(), srvTLSConf); e != nil {
			log.Print(e)
		}
	}()

	cliTLSConf, err := cert.MakeClientTLSConfig(ct.RootCert, ct.ClientCert, ct.ClientKey)
	if err != nil {
		return nil, nil, err
	}
	tc := NewTunnelClient(lis.Addr().String(), cliTLSConf)
	return ts, tc, nil
}

func TestH2Tunnel(t *testing.T) {
	echo, err := ynet.StartEchoServer()
	if err != nil {
		t.Fatal(err)
	}
	defer echo.Close()
	ts, tc, err := startTunnel()
	if err != nil {
		t.Fatal(err)
	}
	defer ts.Close()

	time.Sleep(time.Millisecond * 100)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	rwc, err := tc.DialContext(ctx, echo.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer rwc.Close()

	want := []byte{1}
	got := make([]byte, len(want))
	if _, we := rwc.Write(want); we != nil {
		t.Fatalf("write data: %s", we)
	}
	n, re := rwc.Read(got)
	if re != nil && re != io.EOF {
		t.Fatalf("read data: %s", re)
	}
	if n != len(want) {
		t.Fatalf("got %d bytes, want %d bytes", n, len(want))
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

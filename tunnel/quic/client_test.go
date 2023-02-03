package quic

import (
	"bytes"
	"context"
	"log"
	"net"
	"testing"
	"time"

	"github.com/chenen3/yeager/cert"
	ynet "github.com/chenen3/yeager/net"
)

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
	tc := NewTunnelClient(lis.Addr().String(), cliTLSConf, 1)
	return ts, tc, nil
}

func TestTunnel(t *testing.T) {
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
	defer tc.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	rwc, err := tc.DialContext(ctx, echo.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer rwc.Close()

	want := []byte{1}
	got := make([]byte, len(want))
	if _, err := rwc.Write(want); err != nil {
		t.Fatal(err)
	}
	if _, err := rwc.Read(got); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func BenchmarkThroughput(b *testing.B) {
	echo, err := ynet.StartEchoServer()
	if err != nil {
		b.Fatal(err)
	}
	defer echo.Close()
	ts, tc, err := startTunnel()
	if err != nil {
		b.Fatal(err)
	}
	defer ts.Close()
	defer tc.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	rwc, err := tc.DialContext(ctx, echo.Listener.Addr().String())
	if err != nil {
		b.Fatal(err)
	}

	const n = 1000
	up := make([]byte, n)
	for i := 0; i < n; i++ {
		up[i] = byte(i)
	}
	down := make([]byte, n)
	start := time.Now()
	b.ResetTimer()
	done := make(chan struct{})
	go func() {
		for i := 0; i < b.N; i++ {
			rwc.Write(up)
		}
		close(done)
	}()
	for i := 0; i < b.N; i++ {
		rwc.Read(down)
	}
	b.StopTimer()
	elapsed := time.Since(start)

	megabits := 8 * n * b.N / 1e6
	b.ReportMetric(float64(megabits)/elapsed.Seconds(), "mbps")

	rwc.Close()
	<-done
}
package http2

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net"
	"testing"
	"time"

	"github.com/chenen3/yeager/cert"
	"github.com/chenen3/yeager/echo"
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
		if e := ts.Serve(lis.Addr().String(), srvTLSConf, "", ""); e != nil {
			slog.Error(e.Error())
		}
	}()

	cliTLSConf, err := cert.MakeClientTLSConfig(ct.RootCert, ct.ClientCert, ct.ClientKey)
	if err != nil {
		return nil, nil, err
	}
	tc := NewTunnelClient(lis.Addr().String(), cliTLSConf, "", "")
	return ts, tc, nil
}

func TestH2Tunnel(t *testing.T) {
	e := echo.NewServer()
	defer e.Close()
	ts, tc, err := startTunnel()
	if err != nil {
		t.Fatal(err)
	}
	defer ts.Close()

	time.Sleep(time.Millisecond * 100)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	rwc, err := tc.DialContext(ctx, e.Listener.Addr().String())
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

func TestAuth(t *testing.T) {
	es := echo.NewServer()
	defer es.Close()

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	lis.Close()
	ct, err := cert.Generate("127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	srvTLSConf, err := cert.MakeServerTLSConfig(ct.RootCert, ct.ServerCert, ct.ServerKey)
	if err != nil {
		t.Fatal(err)
	}
	user, pass := "u", "p"
	ts := new(TunnelServer)
	go func() {
		if e := ts.Serve(lis.Addr().String(), srvTLSConf, user, pass); e != nil {
			slog.Error(e.Error())
		}
	}()
	defer ts.Close()
	cliTLSConf, err := cert.MakeClientTLSConfig(ct.RootCert, ct.ClientCert, ct.ClientKey)
	if err != nil {
		t.Fatal(err)
	}
	tc := NewTunnelClient(lis.Addr().String(), cliTLSConf, user, pass)
	defer tc.Close()

	time.Sleep(time.Millisecond * 100)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	rwc, err := tc.DialContext(ctx, es.Listener.Addr().String())
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

func TestBadAuth(t *testing.T) {
	es := echo.NewServer()
	defer es.Close()

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	lis.Close()
	ct, err := cert.Generate("127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	srvTLSConf, err := cert.MakeServerTLSConfig(ct.RootCert, ct.ServerCert, ct.ServerKey)
	if err != nil {
		t.Fatal(err)
	}
	user, pass := "u", "p"
	ts := new(TunnelServer)
	go func() {
		if e := ts.Serve(lis.Addr().String(), srvTLSConf, user, pass); e != nil {
			slog.Error(e.Error())
		}
	}()
	defer ts.Close()
	cliTLSConf, err := cert.MakeClientTLSConfig(ct.RootCert, ct.ClientCert, ct.ClientKey)
	if err != nil {
		t.Fatal(err)
	}
	tc := NewTunnelClient(lis.Addr().String(), cliTLSConf, "fakeuser", "fakepass")
	defer tc.Close()

	time.Sleep(time.Millisecond * 100)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err = tc.DialContext(ctx, es.Listener.Addr().String())
	if err == nil {
		t.Fatalf("expected error for mismatch auth")
	}

	tc2 := NewTunnelClient(lis.Addr().String(), cliTLSConf, "", "")
	defer tc2.Close()
	time.Sleep(time.Millisecond * 100)
	_, err = tc2.DialContext(ctx, es.Listener.Addr().String())
	if err == nil {
		t.Fatalf("expected error for empty auth")
	}
}

func BenchmarkThroughput(b *testing.B) {
	es := echo.NewServer()
	defer es.Close()

	ts, tc, err := startTunnel()
	if err != nil {
		b.Fatal(err)
	}
	defer ts.Close()
	defer tc.Close()
	// the tunnel server may not started yet
	time.Sleep(time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	rwc, err := tc.DialContext(ctx, es.Listener.Addr().String())
	if err != nil {
		b.Fatal(err)
	}
	defer rwc.Close()

	const n = 1000
	up := make([]byte, n)
	for i := 0; i < n; i++ {
		up[i] = byte(i)
	}
	down := make([]byte, n)
	start := time.Now()
	b.ResetTimer()
	go func() {
		for i := 0; i < b.N; i++ {
			rwc.Write(up)
		}
	}()
	for i := 0; i < b.N; i++ {
		io.ReadFull(rwc, down)
	}
	b.StopTimer()
	elapsed := time.Since(start)

	megabits := 8 * n * b.N / 1e6
	b.ReportMetric(float64(megabits)/elapsed.Seconds(), "mbps")
}

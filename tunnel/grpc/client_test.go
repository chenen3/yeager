package grpc

import (
	"bytes"
	"context"
	"errors"
	"log"
	"net"
	"testing"
	"time"

	"github.com/chenen3/yeager/cert"
	ynet "github.com/chenen3/yeager/net"
)

func startTunnel() (*TunnelServer, *TunnelClient, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, nil, err
	}
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
		e := ts.Serve(listener, srvTLSConf)
		if e != nil && !errors.Is(e, net.ErrClosed) {
			log.Print(err)
		}
	}()

	cliTLSConf, err := cert.MakeClientTLSConfig(ct.RootCert, ct.ClientCert, ct.ClientKey)
	if err != nil {
		return nil, nil, err
	}
	tc := NewTunnelClient(TunnelClientConfig{
		Target:    listener.Addr().String(),
		TLSConfig: cliTLSConf,
	})
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
	// the tunnel server may not started yet
	time.Sleep(time.Millisecond)

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

func TestScale(t *testing.T) {
	echo, err := ynet.StartEchoServer()
	if err != nil {
		t.Fatal(err)
	}
	defer echo.Close()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ct, err := cert.Generate("127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	srvTLSConf, err := cert.MakeServerTLSConfig(ct.RootCert, ct.ServerCert, ct.ServerKey)
	if err != nil {
		t.Fatal(err)
	}
	ts := TunnelServer{idleTimeout: 10 * time.Millisecond}
	go func() {
		e := ts.Serve(listener, srvTLSConf)
		if e != nil && !errors.Is(e, net.ErrClosed) {
			log.Print(err)
		}
	}()
	defer ts.Close()

	cliTLSConf, err := cert.MakeClientTLSConfig(ct.RootCert, ct.ClientCert, ct.ClientKey)
	if err != nil {
		t.Fatal(err)
	}
	tc := NewTunnelClient(TunnelClientConfig{
		Target:            listener.Addr().String(),
		TLSConfig:         cliTLSConf,
		WatchPeriod:       5 * time.Millisecond,
		IdleTimeout:       10 * time.Millisecond,
		MaxStreamsPerConn: 1,
	})
	defer tc.Close()
	// the tunnel server may not started yet
	time.Sleep(time.Millisecond)

	issueConnection := func() {
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
	}
	for i := 0; i < 2; i++ {
		issueConnection()
	}

	// check whether scale down
	for i := 0; i < 5; i++ {
		time.Sleep(tc.conf.WatchPeriod)
		if tc.countConn() == 0 {
			return
		}
	}
	t.Fatalf("got %d connections, want %d", tc.countConn(), 0)
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
	// the tunnel server may not started yet
	time.Sleep(time.Millisecond)

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

// fixed number of connection
// BenchmarkThroughput-4   	   68307	     16693 ns/op	       478.8 mbps	    7052 B/op	      13 allocs/op
// BenchmarkThroughput-4   	   88024	     14909 ns/op	       536.4 mbps	    7034 B/op	      13 allocs/op
// BenchmarkThroughput-4   	   90344	     14411 ns/op	       554.5 mbps	    7014 B/op	      13 allocs/op
// BenchmarkThroughput-4   	   89446	     13127 ns/op	       608.9 mbps	    6985 B/op	      13 allocs/op
// BenchmarkThroughput-4   	   90462	     13039 ns/op	       612.9 mbps	    6992 B/op	      13 allocs/op

// dynamic number of connection
// BenchmarkThroughput-4   	   75130	     17879 ns/op	       447.4 mbps	    7012 B/op	      13 allocs/op
// BenchmarkThroughput-4   	   86664	     15511 ns/op	       515.5 mbps	    7002 B/op	      13 allocs/op
// BenchmarkThroughput-4   	   87466	     13242 ns/op	       603.4 mbps	    6997 B/op	      13 allocs/op
// BenchmarkThroughput-4   	   88327	     13052 ns/op	       612.3 mbps	    6997 B/op	      13 allocs/op

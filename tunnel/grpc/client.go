package grpc

import (
	"context"
	"crypto/tls"
	"expvar"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"github.com/chenen3/yeager/debug"
	ynet "github.com/chenen3/yeager/net"
	"github.com/chenen3/yeager/tunnel"
	"github.com/chenen3/yeager/tunnel/grpc/pb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials"
)

var connCount = new(debug.Counter)

func init() {
	expvar.Publish("conngrpc", connCount)
}

type TunnelClient struct {
	conf        TunnelClientConfig
	mu          sync.RWMutex // guards conns
	conns       []*grpc.ClientConn
	streamCount int32
	ticker      *time.Ticker
}

type TunnelClientConfig struct {
	Target            string
	TLSConfig         *tls.Config
	WatchPeriod       time.Duration // default to 1 minute
	IdleTimeout       time.Duration // default to 2 minutes
	MaxStreamsPerConn int           // default to 100
}

func NewTunnelClient(conf TunnelClientConfig) *TunnelClient {
	if conf.TLSConfig == nil {
		panic("TLS config required")
	}
	conf.TLSConfig.NextProtos = []string{"quic"}
	if conf.WatchPeriod == 0 {
		conf.WatchPeriod = time.Minute
	}
	if conf.IdleTimeout == 0 {
		conf.IdleTimeout = ynet.IdleTimeout
	}
	if conf.MaxStreamsPerConn <= 0 {
		conf.MaxStreamsPerConn = maxConcurrentStreams
	}

	c := &TunnelClient{
		conf:   conf,
		ticker: time.NewTicker(conf.WatchPeriod),
	}
	go c.watch()
	connCount.Register(c.countConn)
	return c
}

func (c *TunnelClient) watch() {
	for range c.ticker.C {
		c.mu.Lock()
		c.clearConnectionLocked()
		c.mu.Unlock()
	}
}

func (c *TunnelClient) clearConnectionLocked() {
	if len(c.conns) == 0 {
		return
	}

	live := make([]*grpc.ClientConn, 0, len(c.conns))
	for _, conn := range c.conns {
		// grpc-go does not implement idle timeout on the client side,
		// when the server connection idle timeout and sends GO_AWAY,
		// ClientConn will reconnect and idle.
		if conn.GetState() == connectivity.Idle {
			conn.Close()
			continue
		}
		live = append(live, conn)
	}
	if len(live) < len(c.conns) {
		c.conns = live
		debug.Logf("scale down to %d connection", len(live))
	}
}

func (c *TunnelClient) getConn() (*grpc.ClientConn, error) {
	i := int(atomic.LoadInt32(&c.streamCount)) / c.conf.MaxStreamsPerConn
	c.mu.RLock()
	if i < len(c.conns) && c.conns[i].GetState() != connectivity.Shutdown {
		conn := c.conns[i]
		c.mu.RUnlock()
		return conn, nil
	}
	c.mu.RUnlock()

	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(credentials.NewTLS(c.conf.TLSConfig)),
		// grpc.WithKeepaliveParams(keepalive.ClientParameters{
		// 	Time:    ynet.KeepAlivePeriod,
		// 	Timeout: 1 * time.Second,
		// }),
		grpc.WithConnectParams(grpc.ConnectParams{
			Backoff: backoff.Config{
				BaseDelay:  1.0 * time.Second,
				Multiplier: 1.6,
				Jitter:     0.2,
				MaxDelay:   20 * time.Second,
			},
			MinConnectTimeout: 5 * time.Second,
		}),
	}
	// non-blocking dial, no context required
	conn, err := grpc.Dial(c.conf.Target, opts...)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if i >= len(c.conns) {
		c.conns = append(c.conns, conn)
		debug.Logf("scale up to %d connection", len(c.conns))
		return conn, nil
	}

	if c.conns[i].GetState() != connectivity.Shutdown {
		conn.Close()
		return c.conns[i], nil
	}

	c.conns[i] = conn
	return conn, nil
}

func (c *TunnelClient) DialContext(ctx context.Context, dst string) (io.ReadWriteCloser, error) {
	conn, err := c.getConn()
	if err != nil {
		return nil, fmt.Errorf("connect grpc: %s", err)
	}
	client := pb.NewTunnelClient(conn)

	// requires a context for controling the entire lifecycle of stream, not for dialing
	streamCtx, streamCancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			streamCancel()
		case <-done:
		}
	}()
	defer close(done)

	stream, err := client.Stream(streamCtx)
	if err != nil {
		streamCancel()
		return nil, fmt.Errorf("create grpc stream: %s", err)
	}

	atomic.AddInt32(&c.streamCount, 1)
	sw := wrapClientStream(stream, func() {
		streamCancel()
		atomic.AddInt32(&c.streamCount, -1)
	})
	if err := tunnel.WriteHeader(sw, dst); err != nil {
		sw.Close()
		return nil, err
	}
	return sw, nil
}

func (c *TunnelClient) countConn() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.conns)
}

func (c *TunnelClient) Close() error {
	if c.ticker != nil {
		c.ticker.Stop()
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, conn := range c.conns {
		if conn != nil {
			conn.Close()
		}
	}
	c.conns = nil
	return nil
}

var _ io.ReadWriteCloser = (*clientStreamWrapper)(nil)

type clientStreamWrapper struct {
	stream  pb.Tunnel_StreamClient
	once    sync.Once
	onClose func()
	buf     []byte
	off     int
}

func wrapClientStream(stream pb.Tunnel_StreamClient, onClose func()) *clientStreamWrapper {
	return &clientStreamWrapper{stream: stream, onClose: onClose}
}

func (sw *clientStreamWrapper) Read(b []byte) (n int, err error) {
	if sw.off >= len(sw.buf) {
		data, err := sw.stream.Recv()
		if err != nil {
			return 0, err
		}
		sw.buf = data.Data
		sw.off = 0
	}
	n = copy(b, sw.buf[sw.off:])
	sw.off += n
	return n, nil
}

// Write wraps method Send of client side stream, which is SendMsg actually.
// according to gRPC doc:
//
//	SendMsg does not wait until the message is received by the server. An
//	untimely stream closure may result in lost messages. To ensure delivery,
//	users should ensure the RPC completed successfully using RecvMsg.
func (sw *clientStreamWrapper) Write(b []byte) (n int, err error) {
	err = sw.stream.Send(&pb.Data{Data: b})
	if err != nil {
		return 0, err
	}
	return len(b), nil
}

func (sw *clientStreamWrapper) Close() error {
	if sw.onClose != nil {
		// caller may call Close() twice,
		// which will result in an incorrect streamCount
		sw.once.Do(sw.onClose)
	}
	return nil
}

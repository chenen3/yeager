package grpc

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/chenen3/yeager/tunnel"
	"github.com/chenen3/yeager/tunnel/grpc/pb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials"
)

type TunnelClient struct {
	conf   TunnelClientConfig
	mu     sync.RWMutex // guards conns
	conns  map[string]*grpc.ClientConn
	ticker *time.Ticker
}

type TunnelClientConfig struct {
	Target      string
	TLSConfig   *tls.Config
	watchPeriod time.Duration // default to 1 minute
}

func NewTunnelClient(conf TunnelClientConfig) *TunnelClient {
	if conf.TLSConfig == nil {
		panic("TLS config required")
	}
	if conf.watchPeriod == 0 {
		conf.watchPeriod = time.Minute
	}

	c := &TunnelClient{
		conf:   conf,
		ticker: time.NewTicker(conf.watchPeriod),
		conns:  make(map[string]*grpc.ClientConn),
	}
	go c.watch()
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

	for key, conn := range c.conns {
		// grpc-go does not implement idle timeout on the client side,
		// when the server connection idle timeout and sends GO_AWAY,
		// ClientConn will reconnect and idle.
		if conn.GetState() == connectivity.Idle {
			conn.Close()
			delete(c.conns, key)
		}
	}
}

func (c *TunnelClient) getConn(dst string) (*grpc.ClientConn, error) {
	c.mu.RLock()
	conn, ok := c.conns[dst]
	c.mu.RUnlock()
	if ok && conn.GetState() != connectivity.Shutdown {
		return conn, nil
	}

	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(credentials.NewTLS(c.conf.TLSConfig)),
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
	newconn, err := grpc.Dial(c.conf.Target, opts...)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c, ok := c.conns[dst]; ok && c.GetState() != connectivity.Shutdown {
		newconn.Close()
		return c, nil
	}
	c.conns[dst] = newconn
	return newconn, nil
}

func (c *TunnelClient) DialContext(ctx context.Context, dst string) (io.ReadWriteCloser, error) {
	conn, err := c.getConn(dst)
	if err != nil {
		return nil, fmt.Errorf("connect grpc: %s", err)
	}
	client := pb.NewTunnelClient(conn)

	// the context controls the entire lifecycle of stream
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

	sw := wrapClientStream(stream, streamCancel)
	if err := tunnel.WriteHeader(sw, dst); err != nil {
		sw.Close()
		return nil, err
	}
	return sw, nil
}

func (c *TunnelClient) CountConn() int {
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

func (sw *clientStreamWrapper) Write(b []byte) (n int, err error) {
	err = sw.stream.Send(&pb.Data{Data: b})
	if err != nil {
		return 0, err
	}
	return len(b), nil
}

func (sw *clientStreamWrapper) Close() error {
	if sw.onClose != nil {
		// caller may call Close() twice
		sw.once.Do(sw.onClose)
	}
	return nil
}

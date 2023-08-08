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
	addr      string
	cfg       *tls.Config
	mu        sync.Mutex
	conns     map[string]*grpc.ClientConn
	stats     map[string]int     // number of streams on ClientConn
	pending   []*grpc.ClientConn // named pending to distinguish it from the idle mechanism of grpc
	pendingAt []time.Time
}

func NewTunnelClient(addr string, cfg *tls.Config) *TunnelClient {
	return &TunnelClient{
		addr:  addr,
		cfg:   cfg,
		conns: make(map[string]*grpc.ClientConn),
		stats: make(map[string]int),
	}
}

func canTakeRequest(cc *grpc.ClientConn) bool {
	s := cc.GetState()
	return s == connectivity.Connecting || s == connectivity.Ready
	// ClientConn closes all connections when becoming idle due to WithIdleTimeout option,
	// so cannot take request.
}

func (c *TunnelClient) putConn(key string, cc *grpc.ClientConn) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stats[key]--
	if c.stats[key] == 0 {
		delete(c.conns, key)
		delete(c.stats, key)
		c.pending = append(c.pending, cc)
		c.pendingAt = append(c.pendingAt, time.Now())
	}
}

// getConn tends to use existing client connections, dialing new ones if necessary.
func (c *TunnelClient) getConn(ctx context.Context, key string) (*grpc.ClientConn, error) {
	c.mu.Lock()
	if cc, ok := c.conns[key]; ok {
		if canTakeRequest(cc) {
			c.stats[key]++
			c.mu.Unlock()
			return cc, nil
		}
		cc.Close()
		delete(c.conns, key)
	}

	if len(c.pending) > 0 {
		for i, cc := range c.pending {
			if time.Since(c.pendingAt[i]) < idleTimeout && canTakeRequest(cc) {
				c.conns[key] = cc
				c.stats[key]++
				c.pending = c.pending[i+1:]
				c.pendingAt = c.pendingAt[i+1:]
				c.mu.Unlock()
				return cc, nil
			}
			cc.Close()
		}
		c.pending = nil
		c.pendingAt = nil
	}
	c.mu.Unlock()

	return c.addConn(ctx, key)
}

func (c *TunnelClient) addConn(ctx context.Context, key string) (*grpc.ClientConn, error) {
	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(credentials.NewTLS(c.cfg)),
		grpc.WithConnectParams(grpc.ConnectParams{
			Backoff: backoff.Config{
				BaseDelay:  1.0 * time.Second,
				Multiplier: 1.5,
				Jitter:     0.2,
				MaxDelay:   5 * time.Second,
			},
			MinConnectTimeout: 5 * time.Second,
		}),
		// blocking dial facilitates clear logic while creating stream
		grpc.WithBlock(),
	}
	conn, err := grpc.DialContext(ctx, c.addr, opts...)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if old, ok := c.conns[key]; ok {
		conn.Close()
		c.stats[key]++
		return old, nil
	}
	c.conns[key] = conn
	c.stats[key]++
	return conn, nil
}

func (c *TunnelClient) DialContext(ctx context.Context, dst string) (io.ReadWriteCloser, error) {
	conn, err := c.getConn(ctx, dst)
	if err != nil {
		return nil, fmt.Errorf("connect grpc: %s", err)
	}
	client := pb.NewTunnelClient(conn)
	// the context controls the lifecycle of stream
	sctx, cancel := context.WithCancel(context.Background())
	stream, err := client.Stream(sctx)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("create grpc stream: %s", err)
	}

	rwc := wrapClientStream(stream, func() {
		cancel()
		c.putConn(dst, conn)
	})
	if err := tunnel.WriteHeader(rwc, dst); err != nil {
		rwc.Close()
		return nil, err
	}
	return rwc, nil
}

func (c *TunnelClient) ConnNum() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.conns) + len(c.pending)
}

func (c *TunnelClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, cc := range c.conns {
		cc.Close()
	}
	for _, cc := range c.pending {
		cc.Close()
	}
	return nil
}

var _ io.ReadWriteCloser = (*clientStreamWrapper)(nil)

type clientStreamWrapper struct {
	stream  pb.Tunnel_StreamClient
	onClose func()
	buf     []byte
	off     int
}

// return an stream wrapper that implements io.ReadWriteCloser
func wrapClientStream(stream pb.Tunnel_StreamClient, onClose func()) *clientStreamWrapper {
	return &clientStreamWrapper{stream: stream, onClose: onClose}
}

func (sw *clientStreamWrapper) Read(b []byte) (n int, err error) {
	if sw.off >= len(sw.buf) {
		d, err := sw.stream.Recv()
		if err != nil {
			return 0, err
		}
		sw.buf = d.Data
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
		sw.onClose()
	}
	return nil
}

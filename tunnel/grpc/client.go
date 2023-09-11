package grpc

import (
	"context"
	"crypto/tls"
	"io"
	"sync"
	"time"

	"github.com/chenen3/yeager/tunnel/grpc/pb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/metadata"
)

type TunnelClient struct {
	addr  string
	cfg   *tls.Config
	mu    sync.Mutex
	conns []*grpc.ClientConn
}

func NewTunnelClient(addr string, cfg *tls.Config) *TunnelClient {
	return &TunnelClient{
		addr: addr,
		cfg:  cfg,
	}
}

func canTakeRequest(cc *grpc.ClientConn) bool {
	s := cc.GetState()
	return s != connectivity.Shutdown && s != connectivity.TransientFailure
}

const keepaliveInterval = 15 * time.Second

// getConn tends to use existing client connections, dialing new ones if necessary.
// To mitigate the website fingerprinting via multiplexing in HTTP/2,
// fewer connections will be better.
func (c *TunnelClient) getConn(ctx context.Context) (*grpc.ClientConn, error) {
	c.mu.Lock()
	for i, cc := range c.conns {
		if canTakeRequest(cc) {
			if i > 0 {
				// clear dead conn
				c.conns = c.conns[i:]
			}
			c.mu.Unlock()
			return cc, nil
		}
		cc.Close()
	}
	c.mu.Unlock()

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
		grpc.WithBlock(), // blocking dial facilitates clear logic while creating stream
		grpc.WithIdleTimeout(idleTimeout),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:    keepaliveInterval,
			Timeout: 2 * time.Second,
		}),
	}
	conn, err := grpc.DialContext(ctx, c.addr, opts...)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	c.conns = append(c.conns, conn)
	c.mu.Unlock()
	return conn, nil
}

const targetKey = "target"

func (c *TunnelClient) DialContext(ctx context.Context, dst string) (io.ReadWriteCloser, error) {
	conn, err := c.getConn(ctx)
	if err != nil {
		return nil, err
	}

	client := pb.NewTunnelClient(conn)
	// this context controls the lifetime of the stream, do not use short-lived contexts
	sctx, cancel := context.WithCancel(context.Background())
	sctx = metadata.NewOutgoingContext(sctx, metadata.Pairs(targetKey, dst))
	stream, err := client.Stream(sctx)
	if err != nil {
		cancel()
		conn.Close()
		return nil, err
	}
	return wrapClientStream(stream, cancel), nil
}

func (c *TunnelClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, cc := range c.conns {
		cc.Close()
	}
	return nil
}

var _ io.ReadWriteCloser = (*clientStreamWrapper)(nil)

type clientStreamWrapper struct {
	stream  pb.Tunnel_StreamClient
	onClose func()
	buf     []byte
}

// wraps client stream as io.ReadWriteCloser
func wrapClientStream(stream pb.Tunnel_StreamClient, onClose func()) *clientStreamWrapper {
	return &clientStreamWrapper{stream: stream, onClose: onClose}
}

func (cs *clientStreamWrapper) Read(b []byte) (n int, err error) {
	if len(cs.buf) == 0 {
		m, err := cs.stream.Recv()
		if err != nil {
			return 0, err
		}
		cs.buf = m.Data
	}
	n = copy(b, cs.buf)
	cs.buf = cs.buf[n:]
	return n, nil
}

func (cs *clientStreamWrapper) Write(b []byte) (n int, err error) {
	if err = cs.stream.Send(&pb.Message{Data: b}); err != nil {
		return 0, err
	}
	return len(b), nil
}

func (cs *clientStreamWrapper) Close() error {
	if cs.onClose != nil {
		cs.onClose()
	}
	return nil
}

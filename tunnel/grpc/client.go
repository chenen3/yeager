package grpc

import (
	"context"
	"crypto/tls"
	"errors"
	"expvar"
	"fmt"
	"io"
	"log"
	"sync"
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
	srvAddr string
	tlsConf *tls.Config
	done    chan struct{}

	mu          sync.RWMutex // guards following map
	conns       map[string]*grpc.ClientConn
	startIdle   map[string]time.Time
	watchPeriod time.Duration // for test
	idleTimeout time.Duration // for test
}

func NewTunnelClient(address string, tlsConf *tls.Config) *TunnelClient {
	c := &TunnelClient{
		srvAddr:   address,
		tlsConf:   tlsConf,
		conns:     make(map[string]*grpc.ClientConn),
		startIdle: make(map[string]time.Time),
		done:      make(chan struct{}),
	}
	go c.watch()
	connCount.Register(c.Len)
	return c
}

const defaultWatchPeriod = time.Minute

// watch watches and closes the idle timeout connection.
// grpc-go does not implement the feature on client side,
// see https://github.com/grpc/grpc-go/issues/1719
func (c *TunnelClient) watch() {
	period := c.watchPeriod
	if period == 0 {
		period = defaultWatchPeriod
	}
	idleTimeout := c.idleTimeout
	if idleTimeout == 0 {
		idleTimeout = ynet.IdleTimeout
	}

	ticker := time.NewTicker(period)
	for {
		select {
		case <-c.done:
			ticker.Stop()
			return
		case <-ticker.C:
			c.mu.Lock()
			for key, conn := range c.conns {
				if conn.GetState() != connectivity.Idle {
					delete(c.startIdle, key)
					continue
				}
				t, ok := c.startIdle[key]
				if !ok {
					c.startIdle[key] = time.Now()
					continue
				}
				if time.Since(t) >= idleTimeout {
					conn.Close()
					delete(c.conns, key)
					delete(c.startIdle, key)
					if debug.Enabled() {
						log.Printf("close idle timeout connection: %s", key)
					}
				}
			}
			c.mu.Unlock()
		}
	}
}

func (c *TunnelClient) getConn(addr string) (*grpc.ClientConn, error) {
	c.mu.RLock()
	conn, ok := c.conns[addr]
	c.mu.RUnlock()
	if ok {
		if conn.GetState() == connectivity.Shutdown {
			return nil, errors.New("dead connection")
		}
		return conn, nil
	}

	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(credentials.NewTLS(c.tlsConf)),
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
	// non-blocking dial
	newConn, err := grpc.Dial(c.srvAddr, opts...)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if conn, ok := c.conns[addr]; ok {
		newConn.Close()
		return conn, nil
	}
	c.conns[addr] = newConn
	return newConn, nil
}

func (c *TunnelClient) DialContext(ctx context.Context, dst string) (io.ReadWriteCloser, error) {
	conn, err := c.getConn(dst)
	if err != nil {
		return nil, fmt.Errorf("connect grpc: %s", err)
	}
	client := pb.NewTunnelClient(conn)

	// requires a context for controling the entire lifecycle of stream, not for dialing
	streamCtx, streamCancel := context.WithCancel(context.Background())
	doneOpeningStream := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			streamCancel()
		case <-doneOpeningStream:
		}
	}()
	defer close(doneOpeningStream)

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

func (c *TunnelClient) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.conns)
}

func (c *TunnelClient) Close() error {
	close(c.done)
	c.mu.Lock()
	defer c.mu.Unlock()
	for key, conn := range c.conns {
		conn.Close()
		delete(c.conns, key)
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
		sw.onClose()
	}
	return nil
}

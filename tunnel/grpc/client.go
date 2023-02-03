package grpc

import (
	"context"
	"crypto/tls"
	"errors"
	"expvar"
	"fmt"
	"io"
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
	"google.golang.org/grpc/keepalive"
)

var connCount = new(debug.Counter)

func init() {
	expvar.Publish("conngrpc", connCount)
}

type TunnelClient struct {
	srvAddr string
	tlsConf *tls.Config
	mu      sync.RWMutex // guards conns
	conns   []*grpc.ClientConn
}

// 如何预估所需grpc连接数目：
//
//	每个 gRPC connection 可能使用多个 HTTP/2 连接，连接的数量基于该服务器解析的IP数量，
//	每个连接通常限制 100 个并发的 stream (可以用 MaxConcurrentStreams 修改)
//	假设目标服务器只有 1 个IP，gRPC connection 使用 1 条连接，平均每条连接处理 50 个并发请求，
//	需要的 connection 数量是 ceil(并发请求数 / 50)
//	例如预估有 100 个并发请求，需要 ceil(100 / 50) == 2 个 connection，连接池大小为 2
const defaultConnNum = 2

func NewTunnelClient(address string, tlsConf *tls.Config, connNum int) *TunnelClient {
	if connNum <= defaultConnNum {
		connNum = defaultConnNum
	}
	c := &TunnelClient{
		srvAddr: address,
		tlsConf: tlsConf,
		conns:   make([]*grpc.ClientConn, connNum),
	}
	connCount.Register(c.countConn)
	return c
}

func (c *TunnelClient) getConn(addr string) (*grpc.ClientConn, error) {
	i := len(addr) % len(c.conns)
	c.mu.RLock()
	conn := c.conns[i]
	c.mu.RUnlock()
	if conn != nil {
		if conn.GetState() == connectivity.Shutdown {
			return nil, errors.New("dead connection")
		}
		return conn, nil
	}

	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(credentials.NewTLS(c.tlsConf)),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:    ynet.KeepAlivePeriod,
			Timeout: 1 * time.Second,
		}),
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
	if conn := c.conns[i]; conn != nil {
		newConn.Close()
		return conn, nil
	}
	c.conns[i] = newConn
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

func (c *TunnelClient) countConn() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	var i int
	for _, conn := range c.conns {
		if conn != nil && conn.GetState() != connectivity.Shutdown {
			i++
		}
	}
	return i
}

func (c *TunnelClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, conn := range c.conns {
		if conn != nil {
			conn.Close()
		}
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

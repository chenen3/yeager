/*
 * refer to gRPC performance best practices:
 *     - reuse gRPC channels (grpc.ClientConn)
 *     - use a pool of gRPC channels
 *     - use keepalive pings
 *
 * https://grpc.io/docs/guides/performance/
 * https://grpc.io/blog/grpc-on-http2/
 */

package grpc

import (
	"context"
	"crypto/tls"
	"net"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"

	"github.com/chenen3/yeager/config"
	"github.com/chenen3/yeager/log"
	"github.com/chenen3/yeager/proxy/common"
	"github.com/chenen3/yeager/proxy/yeager/transport/grpc/pb"
)

type dialer struct {
	tlsConf     *tls.Config
	channelPool *channelPool
	mu          sync.Mutex
}

// NewDialer return a gRPC dialer that implements the transport.ContextDialer interface
func NewDialer(tlsConf *tls.Config) *dialer {
	return &dialer{tlsConf: tlsConf}
}

func (d *dialer) ensureChannelPool(addr string) *channelPool {
	if d.channelPool != nil {
		return d.channelPool
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	// check if another goroutine has setup the pool
	if d.channelPool != nil {
		return d.channelPool
	}

	factory := func() (*grpc.ClientConn, error) {
		opts := []grpc.DialOption{
			grpc.WithTransportCredentials(credentials.NewTLS(d.tlsConf)),
			grpc.WithKeepaliveParams(keepalive.ClientParameters{
				Time:    60 * time.Second,
				Timeout: 1 * time.Second,
			}),
		}
		ctx, cancel := context.WithTimeout(context.Background(), common.DialTimeout)
		defer cancel()
		return grpc.DialContext(ctx, addr, opts...)
	}
	d.channelPool = newChannelPool(config.C().GrpcChannelPoolSize, factory)
	return d.channelPool
}

func (d *dialer) DialContext(ctx context.Context, addr string) (net.Conn, error) {
	pool := d.ensureChannelPool(addr)
	// DialContext 的参数 ctx 时效通常很短，不适合控制 stream 的生命周期，因此新建一个
	ctxS, cancelS := context.WithCancel(context.Background())
	ch := make(chan *streamConn, 1)
	go func(ctxS context.Context, cancelS context.CancelFunc, pool *channelPool, ch chan<- *streamConn) {
		channel := pool.Get()
		if !isAvailable(channel) {
			log.L().Error("unavailable grpc channel")
			cancelS()
			return
		}
		client := pb.NewTunnelClient(channel)
		stream, err := client.Stream(ctxS)
		if err != nil {
			log.L().Errorf("create grpc stream: %s", err)
			cancelS()
			return
		}
		ch <- &streamConn{stream: stream, onClose: cancelS}
	}(ctxS, cancelS, pool, ch)

	select {
	case <-ctx.Done():
		cancelS()
		return nil, ctx.Err()
	case <-ctxS.Done():
		return nil, ctxS.Err()
	case sc := <-ch:
		return sc, nil
	}
}

func (d *dialer) Close() error {
	if d.channelPool != nil {
		return d.channelPool.Close()
	}
	return nil
}
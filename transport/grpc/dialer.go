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

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"

	"github.com/chenen3/yeager/config"
	"github.com/chenen3/yeager/proxy/common"
	"github.com/chenen3/yeager/transport/grpc/pb"
)

type dialer struct {
	tlsConf     *tls.Config
	channelPool *channelPool
	once        sync.Once
}

func NewDialer(config *tls.Config) *dialer {
	return &dialer{tlsConf: config}
}

func (d *dialer) DialContext(ctx context.Context, _ string, addr string) (net.Conn, error) {
	d.once.Do(func() {
		channelFactory := func() (*grpc.ClientConn, error) {
			opts := []grpc.DialOption{
				grpc.WithKeepaliveParams(keepalive.ClientParameters{
					Time:    60 * time.Second,
					Timeout: 1 * time.Second,
				}),
			}
			if d.tlsConf == nil {
				opts = append(opts, grpc.WithInsecure())
			} else {
				opts = append(opts, grpc.WithTransportCredentials(credentials.NewTLS(d.tlsConf)))
			}
			ctx, cancel := context.WithTimeout(context.Background(), common.DialTimeout)
			defer cancel()
			return grpc.DialContext(ctx, addr, opts...)
		}
		d.channelPool = newChannelPool(config.C().GrpcChannelPoolSize, channelFactory)
	})

	// DialContext 的参数 ctx 时效通常很短，不适合控制 stream 的生命周期，因此新建一个
	ctxS, cancelS := context.WithCancel(context.Background())
	ch := make(chan *streamConn, 1)
	go func(ctx context.Context, cancel context.CancelFunc, pool *channelPool, ch chan<- *streamConn) {
		channel := pool.Get()
		if !isAvailable(channel) {
			zap.S().Error("unavailable grpc channel")
			cancel()
			return
		}
		client := pb.NewTunnelClient(channel)
		stream, err := client.Stream(ctx)
		if err != nil {
			zap.S().Errorf("create grpc stream: %s", err)
			cancel()
			return
		}
		ch <- &streamConn{stream: stream, onClose: cancel}
	}(ctxS, cancelS, d.channelPool, ch)

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

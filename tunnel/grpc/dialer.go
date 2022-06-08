/*
 * refer to gRPC performance best practices:
 *     - reuse gRPC connection (grpc.ClientConn)
 *     - use a pool of gRPC connection
 *     - use keepalive pings
 *
 * https://grpc.io/docs/guides/performance/
 * https://grpc.io/blog/grpc-on-http2/
 */

package grpc

import (
	"context"
	"crypto/tls"
	"log"
	"net"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"

	"github.com/chenen3/yeager/tunnel/grpc/pb"
	"github.com/chenen3/yeager/util"
)

type dialer struct {
	tlsConf *tls.Config
	pool    *connPool
}

// NewDialer return a gRPC dialer that implements the tunnel.Dialer interface
func NewDialer(tlsConf *tls.Config, addr string, poolSize int) *dialer {
	d := &dialer{tlsConf: tlsConf}
	dialFunc := func() (*grpc.ClientConn, error) {
		ctx, cancel := context.WithTimeout(context.Background(), util.DialTimeout)
		defer cancel()
		opts := []grpc.DialOption{
			grpc.WithTransportCredentials(credentials.NewTLS(d.tlsConf)),
			grpc.WithKeepaliveParams(keepalive.ClientParameters{
				Time:                60 * time.Second,
				Timeout:             1 * time.Second,
				PermitWithoutStream: true,
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
		return grpc.DialContext(ctx, addr, opts...)
	}
	d.pool = newConnPool(poolSize, dialFunc)
	return d
}

func (d *dialer) DialContext(ctx context.Context) (net.Conn, error) {
	// DialContext 的参数 ctx 时效通常很短，不适合控制 stream 的生命周期，因此新建一个
	ctxS, cancelS := context.WithCancel(context.Background())
	ch := make(chan *clientStreamConn, 1)
	go func() {
		conn, err := d.pool.Get()
		if err != nil {
			log.Printf("get grpc conn: %s", err)
			cancelS()
			return
		}
		client := pb.NewTunnelClient(conn)
		stream, err := client.Stream(ctxS)
		if err != nil {
			log.Printf("create grpc stream: %s", err)
			cancelS()
			return
		}
		ch <- clientStreamAsConn(stream, cancelS)
	}()

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
	if d.pool != nil {
		return d.pool.Close()
	}
	return nil
}

/*
 * refer to gRPC performance best practices:
 *     - reuse gRPC channels (grpc.ClientConn)
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

	"yeager/transport/grpc/pb"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
)

type dialer struct {
	tlsConf *tls.Config
	conn    *grpc.ClientConn
	connMu  sync.Mutex
}

func NewDialer(config *tls.Config) *dialer {
	return &dialer{tlsConf: config}
}

func (d *dialer) DialContext(ctx context.Context, addr string) (net.Conn, error) {
	conn, err := d.grpcDial(addr, ctx)
	if err != nil {
		return nil, err
	}

	client := pb.NewTransportClient(conn)
	// 用来发起连接的参数ctx通常时间很短，而双向流可能会存在一段时间，
	// 因此使用新的context来控制双向流
	ctx2, cancel := context.WithCancel(context.Background())
	stream, err := client.Tunnel(ctx2)
	if err != nil {
		cancel()
		return nil, err
	}

	return newConn(stream, cancel), nil
}

func (d *dialer) grpcDial(addr string, ctx context.Context) (*grpc.ClientConn, error) {
	// optimized
	if d.conn != nil && d.conn.GetState() != connectivity.Shutdown {
		return d.conn, nil
	}

	d.connMu.Lock()
	defer d.connMu.Unlock()
	if d.conn != nil && d.conn.GetState() != connectivity.Shutdown {
		// meanwhile other goroutinue already dial new ClientConn
		return d.conn, nil
	}

	opts := []grpc.DialOption{
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:    60 * time.Second,
			Timeout: 30 * time.Second,
		}),
	}
	if d.tlsConf == nil {
		opts = append(opts, grpc.WithInsecure())
	} else {
		opts = append(opts, grpc.WithTransportCredentials(credentials.NewTLS(d.tlsConf)))
	}
	conn, err := grpc.DialContext(ctx, addr, opts...)
	if err != nil {
		return nil, err
	}

	d.conn = conn
	return conn, nil
}

func (d *dialer) Close() error {
	if d.conn == nil {
		return nil
	}
	return d.conn.Close()
}

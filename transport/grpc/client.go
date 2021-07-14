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
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
	"yeager/transport/grpc/pb"
)

type dialer struct {
	addr   string
	config *tls.Config
	conn   *grpc.ClientConn
}

func NewDialer(addr string, config *tls.Config) *dialer {
	return &dialer{
		addr:   addr,
		config: config,
	}
}

func (d *dialer) grpcDial(ctx context.Context) (*grpc.ClientConn, error) {
	if d.conn != nil && d.conn.GetState() != connectivity.Shutdown {
		return d.conn, nil
	}

	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(credentials.NewTLS(d.config)),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:    60 * time.Second,
			Timeout: 30 * time.Second,
		}),
	}
	conn, err := grpc.DialContext(ctx, d.addr, opts...)
	if err != nil {
		return nil, err
	}

	d.conn = conn
	return d.conn, nil
}

func (d *dialer) DialContext(ctx context.Context) (net.Conn, error) {
	conn, err := d.grpcDial(ctx)
	if err != nil {
		return nil, err
	}

	client := pb.NewTransportClient(conn)
	stream, err := client.Tunnel(context.Background())
	if err != nil {
		return nil, err
	}

	return streamToConn(stream), nil
}

func (d *dialer) Close() error {
	if d.conn == nil {
		return nil
	}
	return d.conn.Close()
}

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
	config *tls.Config
	conn   *grpc.ClientConn
}

func NewDialer(config *tls.Config) *dialer {
	return &dialer{config: config}
}

func (d *dialer) grpcDial(addr string, ctx context.Context) (*grpc.ClientConn, error) {
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
	conn, err := grpc.DialContext(ctx, addr, opts...)
	if err != nil {
		return nil, err
	}

	d.conn = conn
	return d.conn, nil
}

func (d *dialer) DialContext(ctx context.Context, addr string) (net.Conn, error) {
	conn, err := d.grpcDial(addr, ctx)
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

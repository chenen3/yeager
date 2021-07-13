package grpc

import (
	"context"
	"crypto/tls"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials"
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

func (d *dialer) DialContext(ctx context.Context) (net.Conn, error) {
	// TODO: connection pool (https://grpc.io/docs/guides/performance/)
	if d.conn == nil || d.conn.GetState() == connectivity.Shutdown {
		opt := grpc.WithTransportCredentials(credentials.NewTLS(d.config))
		conn, err := grpc.DialContext(ctx, d.addr, opt)
		if err != nil {
			return nil, err
		}
		d.conn = conn
	}

	client := pb.NewTransportClient(d.conn)
	stream, err := client.Tunnel(context.Background())
	if err != nil {
		return nil, err
	}

	return streamToConn(stream), nil
}

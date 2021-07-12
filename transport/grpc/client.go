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
	// TODO: connection pool, reconnect
	// 参考 https://grpc.io/docs/guides/performance/
	clientConn *grpc.ClientConn
}

func NewDialer(addr string, config *tls.Config) *dialer {
	return &dialer{
		addr:   addr,
		config: config,
	}
}

func (d dialer) DialContext(ctx context.Context) (net.Conn, error) {
	if d.clientConn == nil || d.clientConn.GetState() == connectivity.Shutdown {
		opt := grpc.WithTransportCredentials(credentials.NewTLS(d.config))
		conn, err := grpc.DialContext(ctx, d.addr, opt)
		if err != nil {
			return nil, err
		}
		d.clientConn = conn
	}

	client := pb.NewTransportClient(d.clientConn)
	var callOpts []grpc.CallOption
	stream, err := client.Tunnel(context.Background(), callOpts...)
	if err != nil {
		return nil, err
	}

	return streamToConn(stream), nil
}

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

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
	"yeager/transport/grpc/pb"
)

type dialer struct {
	config *tls.Config
	conn   *grpc.ClientConn
	connMu sync.RWMutex
}

func NewDialer(config *tls.Config) *dialer {
	return &dialer{config: config}
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

	return streamToConn(stream, cancel), nil
}

// tryGetClientConn return true if get conn successfully
func (d *dialer) tryGetClientConn() (*grpc.ClientConn, bool) {
	d.connMu.RLock()
	defer d.connMu.RUnlock()
	if d.conn == nil || d.conn.GetState() == connectivity.Shutdown {
		return nil, false
	}
	return d.conn, true
}

// trySetClientConn return true if set conn successfully
func (d *dialer) trySetClientConn(conn *grpc.ClientConn) bool {
	d.connMu.Lock()
	defer d.connMu.Unlock()
	if d.conn != nil && d.conn.GetState() != connectivity.Shutdown {
		return false
	}

	d.conn = conn
	return true
}

func (d *dialer) grpcDial(addr string, ctx context.Context) (*grpc.ClientConn, error) {
	conn, ok := d.tryGetClientConn()
	if ok {
		return conn, nil
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

	ok = d.trySetClientConn(conn)
	if !ok {
		// means that another goroutine has set the grpc connection,
		// this unused connection shall be discard
		conn.Close()
	}

	return d.conn, nil
}

func (d *dialer) Close() error {
	if d.conn == nil {
		return nil
	}
	return d.conn.Close()
}

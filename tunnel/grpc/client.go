package grpc

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"time"

	ynet "github.com/chenen3/yeager/net"
	"github.com/chenen3/yeager/tunnel"
	"github.com/chenen3/yeager/tunnel/grpc/pb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
)

type TunnelClient struct {
	connPool *connPool
}

func NewTunnelClient(address string, tlsConf *tls.Config, poolSize int) *TunnelClient {
	dialFunc := func() (*grpc.ClientConn, error) {
		opts := []grpc.DialOption{
			grpc.WithTransportCredentials(credentials.NewTLS(tlsConf)),
			grpc.WithKeepaliveParams(keepalive.ClientParameters{
				Time:    ynet.KeepAlive,
				Timeout: 1 * time.Second,
				// PermitWithoutStream: true,
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
		return grpc.Dial(address, opts...)
	}
	return &TunnelClient{
		connPool: newConnPool(poolSize, dialFunc),
	}
}

func (c *TunnelClient) DialContext(ctx context.Context, dst string) (io.ReadWriteCloser, error) {
	conn, err := c.connPool.Get()
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

func (c *TunnelClient) Close() error {
	if c.connPool == nil {
		return nil
	}
	return c.connPool.Close()
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
	return len(b), err
}

func (sw *clientStreamWrapper) Close() error {
	if sw.onClose != nil {
		sw.onClose()
	}
	return nil
}

package grpc

import (
	"context"
	"crypto/tls"
	"io"
	"log"
	"time"

	"github.com/chenen3/yeager/tunnel"
	"github.com/chenen3/yeager/tunnel/grpc/pb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
)

type TunnelClient struct {
	pool *pool
}

func NewTunnelClient(address string, tlsConf *tls.Config, poolSize int) *TunnelClient {
	dialFunc := func() (*grpc.ClientConn, error) {
		opts := []grpc.DialOption{
			grpc.WithTransportCredentials(credentials.NewTLS(tlsConf)),
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
		// non-blocking dial
		return grpc.Dial(address, opts...)
	}
	p := newPool(poolSize, dialFunc)
	return &TunnelClient{pool: p}
}

func (c *TunnelClient) DialContext(ctx context.Context, addr string) (io.ReadWriteCloser, error) {
	ch := make(chan pb.Tunnel_StreamClient, 1)
	// requires a context for controling the entire lifecycle of stream, not for dialing
	ctxS, cancelS := context.WithCancel(context.Background())
	go func() {
		conn, err := c.pool.Get()
		if err != nil {
			log.Printf("connect grpc: %s", err)
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
		ch <- stream
	}()
	var stream pb.Tunnel_StreamClient
	select {
	case <-ctx.Done():
		cancelS()
		return nil, ctx.Err()
	case <-ctxS.Done():
		return nil, ctxS.Err()
	case stream = <-ch:
	}

	closeStream := cancelS
	header, err := tunnel.MakeHeader(addr)
	if err != nil {
		closeStream()
		return nil, err
	}
	err = stream.Send(&pb.Data{Data: header})
	if err != nil {
		closeStream()
		return nil, err
	}
	return wrapClientStream(stream, closeStream), nil
}

func (c *TunnelClient) Close() error {
	if c.pool == nil {
		return nil
	}
	return c.pool.Close()
}

var _ io.ReadWriteCloser = (*rwcClientStream)(nil)

type rwcClientStream struct {
	stream  pb.Tunnel_StreamClient
	onClose func()
	buf     []byte
	off     int
}

func wrapClientStream(stream pb.Tunnel_StreamClient, onClose func()) *rwcClientStream {
	return &rwcClientStream{stream: stream, onClose: onClose}
}

func (c *rwcClientStream) Read(b []byte) (n int, err error) {
	if c.off >= len(c.buf) {
		data, err := c.stream.Recv()
		if err != nil {
			return 0, err
		}
		c.buf = data.Data
		c.off = 0
	}
	n = copy(b, c.buf[c.off:])
	c.off += n
	return n, nil
}

// Write wraps method Send of client side stream, which is SendMsg actually.
// according to gRPC doc:
//
//	SendMsg does not wait until the message is received by the server. An
//	untimely stream closure may result in lost messages. To ensure delivery,
//	users should ensure the RPC completed successfully using RecvMsg.
func (c *rwcClientStream) Write(b []byte) (n int, err error) {
	err = c.stream.Send(&pb.Data{Data: b})
	return len(b), err
}

func (c *rwcClientStream) Close() error {
	if c.onClose != nil {
		c.onClose()
	}
	return nil
}

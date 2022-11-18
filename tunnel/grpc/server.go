package grpc

import (
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"

	"github.com/chenen3/yeager/tunnel"
	"github.com/chenen3/yeager/tunnel/grpc/pb"
	"github.com/chenen3/yeager/util"
)

type TunnelServer struct {
	pb.UnimplementedTunnelServer
	mu sync.Mutex
	gs *grpc.Server
}

// Serve will return a non-nil error unless Close is called.
func (s *TunnelServer) Serve(address string, tlsConf *tls.Config) error {
	lis, err := net.Listen("tcp", address)
	if err != nil {
		return err
	}
	// method grpc.Server.Serve() will close the listener

	grpcServer := grpc.NewServer(
		grpc.Creds(credentials.NewTLS(tlsConf)),
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			MinTime:             60 * time.Second,
			PermitWithoutStream: true,
		}),
		grpc.KeepaliveParams(keepalive.ServerParameters{
			MaxConnectionIdle: util.MaxConnectionIdle,
			Time:              60 * time.Second,
			Timeout:           1 * time.Second,
		}),
	)
	pb.RegisterTunnelServer(grpcServer, s)
	s.mu.Lock()
	s.gs = grpcServer
	s.mu.Unlock()
	return grpcServer.Serve(lis)
}

func (s *TunnelServer) Stream(stream pb.Tunnel_StreamServer) error {
	if stream.Context().Err() != nil {
		return stream.Context().Err()
	}

	rwStream := wrapServerStream(stream)
	addr, err := tunnel.TimeReadHeader(rwStream, util.HandshakeTimeout)
	if err != nil {
		return fmt.Errorf("read header: %s", err)
	}

	conn, err := net.DialTimeout("tcp", addr, util.DialTimeout)
	if err != nil {
		return err
	}
	defer conn.Close()

	go io.Copy(conn, rwStream)
	io.Copy(rwStream, conn)
	return nil
}

func (s *TunnelServer) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.gs != nil {
		// will close all active connections and listener
		s.gs.Stop()
	}
	return nil
}

var _ io.ReadWriter = (*rwServerStream)(nil)

type rwServerStream struct {
	stream pb.Tunnel_StreamServer
	buf    []byte
	off    int
}

func wrapServerStream(stream pb.Tunnel_StreamServer) *rwServerStream {
	return &rwServerStream{stream: stream}
}

func (c *rwServerStream) Read(b []byte) (n int, err error) {
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

func (c *rwServerStream) Write(b []byte) (n int, err error) {
	err = c.stream.Send(&pb.Data{Data: b})
	return len(b), err
}

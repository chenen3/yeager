package grpc

import (
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net"
	"sync"

	ynet "github.com/chenen3/yeager/net"
	"github.com/chenen3/yeager/tunnel"
	"github.com/chenen3/yeager/tunnel/grpc/pb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
)

// TunnelServer is a GRPC tunnel server, its zero value is ready to use
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
	// no need to hold the listener, it will be closed by grpc.Server

	grpcServer := grpc.NewServer(
		grpc.Creds(credentials.NewTLS(tlsConf)),
		grpc.KeepaliveParams(keepalive.ServerParameters{
			MaxConnectionIdle: ynet.IdleConnTimeout,
		}),
	)
	pb.RegisterTunnelServer(grpcServer, s)
	s.mu.Lock()
	s.gs = grpcServer
	s.mu.Unlock()
	return grpcServer.Serve(lis)
}

func (s *TunnelServer) Stream(rawStream pb.Tunnel_StreamServer) error {
	if rawStream.Context().Err() != nil {
		return rawStream.Context().Err()
	}

	stream := wrapServerStream(rawStream)
	dst, err := tunnel.TimeReadHeader(stream, ynet.HandshakeTimeout)
	if err != nil {
		return fmt.Errorf("read header: %s", err)
	}

	remote, err := net.DialTimeout("tcp", dst, ynet.DialTimeout)
	if err != nil {
		return err
	}
	defer remote.Close()

	f := ynet.NewForwarder(stream, remote)
	go f.FromClient()
	go f.ToClient()
	if err := <-f.C; err != nil {
		log.Printf("forward %s: %s", dst, err)
	}
	return nil
}

func (s *TunnelServer) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.gs == nil {
		return nil
	}
	// stopping GRPC server will close all active connections and listener
	s.gs.Stop()
	return nil
}

var _ io.ReadWriter = (*serverStreamWrapper)(nil)

type serverStreamWrapper struct {
	stream pb.Tunnel_StreamServer
	buf    []byte
	off    int
}

func wrapServerStream(stream pb.Tunnel_StreamServer) *serverStreamWrapper {
	return &serverStreamWrapper{stream: stream}
}

func (c *serverStreamWrapper) Read(b []byte) (n int, err error) {
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

func (c *serverStreamWrapper) Write(b []byte) (n int, err error) {
	err = c.stream.Send(&pb.Data{Data: b})
	return len(b), err
}

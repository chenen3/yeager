package grpc

import (
	"crypto/tls"
	"errors"
	"io"
	"net"
	"sync"
	"time"

	"github.com/chenen3/yeager/flow"
	"github.com/chenen3/yeager/transport/grpc/pb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/metadata"
)

const idleTimeout = 10 * time.Minute

// Server is a GRPC tunnel server, its zero value is ready to use
type Server struct {
	pb.UnimplementedTunnelServer
	mu sync.Mutex
	gs *grpc.Server
}

// Serve serves incomming connections on listener, blocking until listener fails.
// Serve will return a non-nil error unless Close is called.
func (s *Server) Serve(listener net.Listener, tlsConf *tls.Config) error {
	grpcServer := grpc.NewServer(
		grpc.Creds(credentials.NewTLS(tlsConf)),
		grpc.KeepaliveParams(keepalive.ServerParameters{
			MaxConnectionIdle: idleTimeout,
		}),
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			MinTime: keepaliveInterval,
		}),
	)
	pb.RegisterTunnelServer(grpcServer, s)
	s.mu.Lock()
	s.gs = grpcServer
	s.mu.Unlock()
	return grpcServer.Serve(listener)
}

func (s *Server) Stream(stream pb.Tunnel_StreamServer) error {
	if stream.Context().Err() != nil {
		return stream.Context().Err()
	}
	targets := metadata.ValueFromIncomingContext(stream.Context(), targetKey)
	if len(targets) == 0 {
		return errors.New("missing target")
	}

	targetConn, err := net.DialTimeout("tcp", targets[0], 5*time.Second)
	if err != nil {
		return err
	}
	defer targetConn.Close()

	streamRW := toReadWriter(stream)
	go func() {
		flow.Copy(targetConn, streamRW)
		targetConn.(*net.TCPConn).CloseWrite()
	}()
	flow.Copy(streamRW, targetConn)
	return nil
}

func (s *Server) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.gs == nil {
		return nil
	}
	// stopping GRPC server will close all active connections and listener
	s.gs.Stop()
	return nil
}

type serverStreamRW struct {
	Stream pb.Tunnel_StreamServer
	buf    []byte
}

func toReadWriter(stream pb.Tunnel_StreamServer) io.ReadWriter {
	return &serverStreamRW{Stream: stream}
}

func (ss *serverStreamRW) Read(b []byte) (n int, err error) {
	if len(ss.buf) == 0 {
		m, err := ss.Stream.Recv()
		if err != nil {
			return 0, err
		}
		ss.buf = m.Data
	}
	n = copy(b, ss.buf)
	ss.buf = ss.buf[n:]
	return n, nil
}

func (ss *serverStreamRW) Write(b []byte) (n int, err error) {
	if err = ss.Stream.Send(&pb.Message{Data: b}); err != nil {
		return 0, err
	}
	return len(b), nil
}

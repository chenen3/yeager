package grpc

import (
	"crypto/tls"
	"errors"
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

// Serve always return a non-nil error unless Close is called.
func (s *Server) Serve(lis net.Listener, tlsConf *tls.Config) error {
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
	return grpcServer.Serve(lis)
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

	// err = flow.Relay(&serverStream{Stream: stream}, targetConn)
	// if err != nil && !canIgnore(err) {
	// 	slog.Error.Print(err.Error(), "addr", target)
	// }

	ss := &serverStream{Stream: stream}
	go func() {
		flow.Copy(targetConn, ss)
		tcpConn, _ := targetConn.(*net.TCPConn)
		tcpConn.CloseWrite()
	}()
	flow.Copy(ss, targetConn)
	return nil
}

// func canIgnore(err error) bool {
// 	if errors.Is(err, net.ErrClosed) {
// 		return true
// 	}
// 	if s, ok := status.FromError(err); ok && s.Code() == codes.Canceled {
// 		return true
// 	}
// 	return false
// }

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

type serverStream struct {
	Stream pb.Tunnel_StreamServer
	buf    []byte
}

func (ss *serverStream) Read(b []byte) (n int, err error) {
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

func (ss *serverStream) Write(b []byte) (n int, err error) {
	if err = ss.Stream.Send(&pb.Message{Data: b}); err != nil {
		return 0, err
	}
	return len(b), nil
}

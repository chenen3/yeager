package grpc

import (
	"crypto/tls"
	"errors"
	"io"
	"log/slog"
	"net"
	"sync"
	"time"

	ynet "github.com/chenen3/yeager/net"
	"github.com/chenen3/yeager/tunnel/grpc/pb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const idleTimeout = 10 * time.Minute

// TunnelServer is a GRPC tunnel server, its zero value is ready to use
type TunnelServer struct {
	pb.UnimplementedTunnelServer
	mu sync.Mutex
	gs *grpc.Server
}

// Serve will return a non-nil error unless Close is called.
func (s *TunnelServer) Serve(lis net.Listener, tlsConf *tls.Config) error {
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

func (s *TunnelServer) Stream(stream pb.Tunnel_StreamServer) error {
	if stream.Context().Err() != nil {
		return stream.Context().Err()
	}
	v := metadata.ValueFromIncomingContext(stream.Context(), targetKey)
	if len(v) == 0 {
		return errors.New("empty target")
	}
	target := v[0]

	remote, err := net.DialTimeout("tcp", target, ynet.DialTimeout)
	if err != nil {
		return err
	}
	defer remote.Close()

	err = ynet.Relay(wrapServerStream(stream), remote)
	if err != nil {
		if errors.Is(err, net.ErrClosed) {
			return nil
		}
		if s, ok := status.FromError(err); ok && s.Code() == codes.Canceled {
			return nil
		}
		slog.Error(err.Error(), "addr", target)
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
}

// wraps server stream as io.ReadWriter
func wrapServerStream(stream pb.Tunnel_StreamServer) *serverStreamWrapper {
	return &serverStreamWrapper{stream: stream}
}

func (ss *serverStreamWrapper) Read(b []byte) (n int, err error) {
	if len(ss.buf) == 0 {
		m, err := ss.stream.Recv()
		if err != nil {
			return 0, err
		}
		ss.buf = m.Data
	}
	n = copy(b, ss.buf)
	ss.buf = ss.buf[n:]
	return n, nil
}

func (ss *serverStreamWrapper) Write(b []byte) (n int, err error) {
	if err = ss.stream.Send(&pb.Message{Data: b}); err != nil {
		return 0, err
	}
	return len(b), nil
}

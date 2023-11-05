package grpc

import (
	"crypto/tls"
	"errors"
	"io"
	"net"
	"sync"
	"time"

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
		io.Copy(targetConn, streamRW)
		targetConn.(*net.TCPConn).CloseWrite()
	}()
	io.Copy(streamRW, targetConn)
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
	stream pb.Tunnel_StreamServer
	buf    []byte
}

func toReadWriter(stream pb.Tunnel_StreamServer) io.ReadWriter {
	return &serverStreamRW{stream: stream}
}

func (ss *serverStreamRW) Read(b []byte) (n int, err error) {
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

// WriteTo implements io.WriterTo.
func (ss *serverStreamRW) WriteTo(w io.Writer) (written int64, err error) {
	for {
		msg, er := ss.stream.Recv()
		if msg != nil && len(msg.Data) > 0 {
			nr := len(msg.Data)
			nw, ew := w.Write(msg.Data)
			if nw < 0 || nr < nw {
				nw = 0
				if ew == nil {
					ew = errors.New("invalid write result")
				}
			}
			written += int64(nw)
			if ew != nil {
				err = ew
				break
			}
			if nr != nw {
				err = io.ErrShortWrite
				break
			}
		}
		if er != nil {
			if er != io.EOF {
				err = er
			}
			break
		}
	}
	return written, err
}

func (ss *serverStreamRW) Write(b []byte) (n int, err error) {
	if err = ss.stream.Send(&pb.Message{Data: b}); err != nil {
		return 0, err
	}
	return len(b), nil
}

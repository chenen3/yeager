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

	ss := &serverStream{stream}
	go func() {
		ss.WriteTo(targetConn)
		targetConn.(*net.TCPConn).CloseWrite()
	}()
	ss.ReadFrom(targetConn)
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

// serverStream implements io.WriterTo and io.ReaderFrom as optimizations
// so copying to or from it can avoid allocating unnecessary buffers.
type serverStream struct {
	pb.Tunnel_StreamServer
}

var _ io.WriterTo = (*serverStream)(nil)
var _ io.ReaderFrom = (*serverStream)(nil)

func (ss *serverStream) WriteTo(w io.Writer) (written int64, err error) {
	for {
		msg, er := ss.Recv()
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

func (ss *serverStream) ReadFrom(r io.Reader) (n int64, err error) {
	buf := bufPool.Get().(*[]byte)
	for {
		nr, er := r.Read(*buf)
		if nr > 0 {
			n += int64(nr)
			ew := ss.Send(&pb.Message{Data: (*buf)[:nr]})
			if ew != nil {
				err = ew
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
	bufPool.Put(buf)
	return n, err
}

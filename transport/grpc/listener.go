package grpc

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"

	"github.com/chenen3/yeager/transport/grpc/pb"
)

// listener implement net.Listener and pb.TransportServer
type listener struct {
	pb.UnimplementedTunnelServer
	addr    net.Addr
	connCh  chan net.Conn
	onClose func() // release resource
}

func newListener() *listener {
	return &listener{
		connCh: make(chan net.Conn, 32),
	}
}

func (l *listener) Stream(stream pb.Tunnel_StreamServer) error {
	if err := stream.Context().Err(); err != nil {
		err = errors.New("client stream closed: " + err.Error())
		zap.S().Warn(err)
		return err
	}

	ctx, cancel := context.WithCancel(stream.Context())
	l.connCh <- &streamConn{stream: stream, onClose: cancel}
	<-ctx.Done()
	return nil
}

func (l *listener) Accept() (net.Conn, error) {
	conn, ok := <-l.connCh
	if !ok {
		return nil, errors.New("grpc service stopped")
	}
	return conn, nil
}

func (l *listener) Close() error {
	l.onClose()
	close(l.connCh)
	return nil
}

func (l *listener) Addr() net.Addr {
	return l.addr
}

// given nil tlsConf, data will be transport in plaintext
func Listen(addr string, tlsConf *tls.Config) (net.Listener, error) {
	tcpListener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, errors.New("failed to listen: " + err.Error())
	}

	opt := []grpc.ServerOption{
		// fix grpc client side error: "code = Unavailable desc = transport is closing"
		grpc.KeepaliveParams(keepalive.ServerParameters{
			MaxConnectionIdle: 5 * time.Minute,
		}),
	}
	if tlsConf != nil {
		opt = append(opt, grpc.Creds(credentials.NewTLS(tlsConf)))
	}

	grpcServer := grpc.NewServer(opt...)
	grpcListener := newListener()
	pb.RegisterTunnelServer(grpcServer, grpcListener)
	go func() {
		err := grpcServer.Serve(tcpListener)
		if err != nil {
			zap.S().Error(err)
		}
	}()

	grpcListener.addr = tcpListener.Addr()
	grpcListener.onClose = grpcServer.Stop
	return grpcListener, nil
}

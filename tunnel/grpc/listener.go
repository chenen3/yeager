package grpc

import (
	"context"
	"crypto/tls"
	"errors"
	"log"
	"net"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"

	"github.com/chenen3/yeager/tunnel/grpc/pb"
	"github.com/chenen3/yeager/util"
)

// listener implements the net.Listener and pb.TransportServer interface
type listener struct {
	pb.UnimplementedTunnelServer
	addr    net.Addr
	connCh  chan net.Conn
	onClose func() // release resource
}

func newListener() *listener {
	return &listener{
		connCh: make(chan net.Conn, 100),
	}
}

func (l *listener) Stream(stream pb.Tunnel_StreamServer) error {
	if err := stream.Context().Err(); err != nil {
		err = errors.New("client stream closed: " + err.Error())
		log.Print(err)
		return err
	}

	ctx, cancel := context.WithCancel(stream.Context())
	l.connCh <- serverStreamAsConn(stream, cancel)
	<-ctx.Done()
	return nil
}

func (l *listener) Accept() (net.Conn, error) {
	conn, ok := <-l.connCh
	if !ok {
		return nil, net.ErrClosed
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

func Listen(addr string, tlsConf *tls.Config) (net.Listener, error) {
	tcpListener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}

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
	grpcListener := newListener()
	pb.RegisterTunnelServer(grpcServer, grpcListener)
	go func() {
		err := grpcServer.Serve(tcpListener)
		if err != nil {
			log.Printf("grpc server exit: %s", err)
		}
	}()

	grpcListener.addr = tcpListener.Addr()
	grpcListener.onClose = grpcServer.Stop
	return grpcListener, nil
}

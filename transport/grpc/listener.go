package grpc

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"time"

	"yeager/log"
	"yeager/transport/grpc/pb"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
)

// listener implement net.Listener and pb.TransportServer
type listener struct {
	pb.UnimplementedTransportServer
	addr    net.Addr
	connCh  chan net.Conn
	onClose func() // release resource
}

func newListener() *listener {
	return &listener{
		connCh: make(chan net.Conn, 32),
	}
}

func (l *listener) Tunnel(stream pb.Transport_TunnelServer) error {
	if err := stream.Context().Err(); err != nil {
		err = errors.New("client stream closed: " + err.Error())
		log.Warn(err)
		return err
	}

	ctx, cancel := context.WithCancel(stream.Context())
	l.connCh <- newConn(stream, cancel)
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

func Listen(addr string, tlsConf *tls.Config) (net.Listener, error) {
	tcpListener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, errors.New("failed to listen: " + err.Error())
	}

	opt := []grpc.ServerOption{
		// fix client side error: "code = Unavailable desc = transport is closing"
		// https://www.evanjones.ca/grpc-is-tricky.html
		grpc.KeepaliveParams(keepalive.ServerParameters{
			Time:    60 * time.Second,
			Timeout: 30 * time.Second,
		}),
	}
	if tlsConf != nil {
		opt = append(opt, grpc.Creds(credentials.NewTLS(tlsConf)))
	}
	grpcServer := grpc.NewServer(opt...)
	grpcListener := newListener()
	pb.RegisterTransportServer(grpcServer, grpcListener)
	go func() {
		err := grpcServer.Serve(tcpListener)
		if err != nil {
			log.Error(err)
		}
	}()

	grpcListener.addr = tcpListener.Addr()
	grpcListener.onClose = grpcServer.Stop
	return grpcListener, nil
}

package grpc

import (
	"context"
	"crypto/tls"
	"errors"
	"net"

	"yeager/log"
	"yeager/transport/grpc/pb"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
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

func (s listener) Tunnel(stream pb.Transport_TunnelServer) error {
	if err := stream.Context().Err(); err != nil {
		err = errors.New("client stream closed: " + err.Error())
		log.Warn(err)
		return err
	}

	ctx, cancel := context.WithCancel(stream.Context())
	s.connCh <- streamToConn(stream, cancel)
	<-ctx.Done()
	return nil
}

func (s listener) Accept() (net.Conn, error) {
	conn, ok := <-s.connCh
	if !ok {
		return nil, errors.New("grpc service stopped")
	}
	return conn, nil
}

func (s listener) Close() error {
	if s.onClose != nil {
		s.onClose()
	}
	close(s.connCh)
	return nil
}

func (s listener) Addr() net.Addr {
	return s.addr
}

func Listen(addr string, tlsConf *tls.Config) (net.Listener, error) {
	tcpListener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, errors.New("failed to listen: " + err.Error())
	}

	var opt []grpc.ServerOption
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

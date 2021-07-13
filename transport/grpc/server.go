package grpc

import (
	"crypto/tls"
	"errors"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"yeager/log"
	"yeager/transport/grpc/pb"
)

// server implement pb.TransportServer, but actually used as net.Listener
type server struct {
	pb.UnimplementedTransportServer
	addr    net.Addr
	connCh  chan net.Conn
	onClose func() // release resource
}

func newServer() *server {
	return &server{
		connCh: make(chan net.Conn, 32),
	}
}

func (s server) Tunnel(stream pb.Transport_TunnelServer) error {
	s.connCh <- streamToConn(stream)
	<-stream.Context().Done()
	return nil
}

func (s server) Accept() (net.Conn, error) {
	conn, ok := <-s.connCh
	if !ok {
		return nil, errors.New("grpc service stopped")
	}
	return conn, nil
}

func (s server) Close() error {
	s.onClose()
	close(s.connCh)
	return nil
}

func (s server) Addr() net.Addr {
	return s.addr
}

func Listen(addr string, tlsConf *tls.Config) (net.Listener, error) {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, errors.New("failed to listen: " + err.Error())
	}

	grpcServer := grpc.NewServer(grpc.Creds(credentials.NewTLS(tlsConf)))
	srv := newServer()
	pb.RegisterTransportServer(grpcServer, srv)
	go func() {
		err := grpcServer.Serve(lis)
		if err != nil {
			log.Error(err)
		}
	}()

	srv.addr = lis.Addr()
	srv.onClose = func() {
		grpcServer.Stop()
	}
	return srv, nil
}

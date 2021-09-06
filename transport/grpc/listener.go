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
	l.connCh <- streamToConn(stream, cancel)
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
		// 修复grpc出现间歇性不可用的问题；
		// 问题复现：笔记本电脑睡眠半小时后重新工作，发现yeager代理网络中断，约20秒之后恢复。
		// 客户端报错 "code = Unavailable desc = transport is closing"，
		// 推测是底层TCP连接关闭（可能是被中间的负载均衡服务器或代理服务器关闭的），
		// 但是客户端与服务端都不知道，导致没有及时重新连接。
		// 因此修改grpc服务端配置，主动关闭空闲超时的连接。
		// 参考:
		// https://github.com/grpc/grpc-go#the-rpc-failed-with-error-code--unavailable-desc--transport-is-closing
		// https://www.codenong.com/52993259/
		grpc.KeepaliveParams(keepalive.ServerParameters{
			MaxConnectionIdle: 5 * time.Minute,
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

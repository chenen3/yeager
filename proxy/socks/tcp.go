// Package socks provides a SOCKS version 5 server implementation.
package socks

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/chenen3/yeager/config"
	"github.com/chenen3/yeager/log"
	"github.com/chenen3/yeager/proxy/common"
	"github.com/chenen3/yeager/util"
)

// TCPServer implements protocol.Inbound interface
type TCPServer struct {
	ctx    context.Context
	cancel context.CancelFunc
	conf   *config.SOCKSProxy
	lis    net.Listener
	wg     sync.WaitGroup // counts active Serve goroutines for graceful close

	ready chan struct{} // imply that server is ready to accept connection, testing only
}

func NewTCPServer(conf *config.SOCKSProxy) (*TCPServer, error) {
	if conf == nil || conf.Listen == "" {
		return nil, errors.New("config missing listening address")
	}

	ctx, cancel := context.WithCancel(context.Background())
	return &TCPServer{
		conf:   conf,
		ready:  make(chan struct{}),
		ctx:    ctx,
		cancel: cancel,
	}, nil
}

func (s *TCPServer) ListenAndServe(handle func(ctx context.Context, conn net.Conn, network, addr string)) error {
	lis, err := net.Listen("tcp", s.conf.Listen)
	if err != nil {
		return fmt.Errorf("socks5 proxy failed to listen, err: %s", err)
	}
	s.lis = lis
	log.L().Infof("socks5 TCP proxy listening %s", s.conf.Listen)

	close(s.ready)
	for {
		conn, err := lis.Accept()
		if err != nil {
			select {
			case <-s.ctx.Done():
				return nil
			default:
			}
			log.L().Warnf(err.Error())
			continue
		}

		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			addr, err := s.handshake(conn)
			if err != nil {
				log.L().Error("handshake: " + err.Error())
				conn.Close()
				return
			}

			// according to SOCKS5 protocol, while desire UDP,
			// keep this TCP connection until closed by peer
			if addr.Network() == "udp" {
				s.holdForUDP(conn)
				return
			}

			handle(s.ctx, conn, addr.Network(), addr.String())
		}()
	}
}

func (s *TCPServer) holdForUDP(conn net.Conn) {
	defer conn.Close()
	done := make(chan struct{})
	go func() {
		for {
			var b [1]byte
			_, err := conn.Read(b[:])
			if err, ok := err.(net.Error); ok && err.Timeout() {
				continue
			}
			close(done)
			return
		}
	}()

	select {
	case <-s.ctx.Done():
	case <-done:
	}
}

func (s *TCPServer) Close() error {
	defer s.wg.Wait()
	s.cancel()
	if s.lis != nil {
		return s.lis.Close()
	}
	return nil
}

func (s *TCPServer) handshake(conn net.Conn) (addr *util.Addr, err error) {
	err = conn.SetDeadline(time.Now().Add(common.HandshakeTimeout))
	if err != nil {
		return
	}
	defer func() {
		er := conn.SetDeadline(time.Time{})
		if er != nil && err == nil {
			err = er
		}
	}()

	return handshake(conn)
}

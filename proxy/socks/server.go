// Package socks provides a SOCKS version 5 server implementation.
package socks

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/chenen3/yeager/log"
	"github.com/chenen3/yeager/proxy/common"
)

// Server implements the proxy.Inbounder interface
type Server struct {
	addr    string
	handler func(ctx context.Context, c net.Conn, addr string)
	lis     net.Listener

	wg     sync.WaitGroup
	ctx    context.Context
	cancel context.CancelFunc
	ready  chan struct{} // imply that server is ready to accept connection, testing only
}

func NewServer(addr string) (*Server, error) {
	if addr == "" {
		return nil, errors.New("empty address")
	}

	ctx, cancel := context.WithCancel(context.Background())
	s := &Server{
		addr:   addr,
		ready:  make(chan struct{}),
		ctx:    ctx,
		cancel: cancel,
	}
	return s, nil
}

func (s *Server) Handle(handler func(ctx context.Context, c net.Conn, addr string)) {
	s.handler = handler
}

func (s *Server) ListenAndServe() error {
	lis, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("socks5 proxy failed to listen, err: %s", err)
	}
	s.lis = lis
	log.Infof("socks5 proxy listening %s", s.addr)

	close(s.ready)
	for {
		conn, err := lis.Accept()
		if err != nil {
			select {
			case <-s.ctx.Done():
				return nil
			default:
			}
			log.Errorf("failed to accept conn: %s", err)
			continue
		}

		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			defer conn.Close()
			addr, err := s.handshake(conn)
			if err != nil {
				log.Errorf("failed to handshake: %s", err)
				conn.Close()
				return
			}

			s.handler(s.ctx, conn, addr)
		}()
	}
}

func (s *Server) Close() error {
	s.cancel()
	var err error
	if s.lis != nil {
		err = s.lis.Close()
	}
	return err
}

func (s *Server) GraceClose() error {
	s.cancel()
	var err error
	if s.lis != nil {
		err = s.lis.Close()
	}
	s.wg.Wait()
	return err
}

func (s *Server) handshake(conn net.Conn) (addr string, err error) {
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

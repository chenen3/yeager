// Package socks provides a SOCKS version 5 server implementation.
package socks

import (
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
	handler common.Handler
	lis     net.Listener

	mu         sync.Mutex
	activeConn map[net.Conn]struct{}
	done       chan struct{}
	ready      chan struct{} // imply that server is ready to accept connection, testing only
}

func NewServer(addr string) (*Server, error) {
	if addr == "" {
		return nil, errors.New("empty address")
	}

	return &Server{
		addr:       addr,
		ready:      make(chan struct{}),
		done:       make(chan struct{}),
		activeConn: make(map[net.Conn]struct{}),
	}, nil
}

func (s *Server) Handle(handler common.Handler) {
	s.handler = handler
}

func (s *Server) ListenAndServe() error {
	lis, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("socks5 proxy failed to listen, err: %s", err)
	}
	s.lis = lis
	log.L().Infof("socks5 proxy listening %s", s.addr)

	close(s.ready)
	for {
		conn, err := lis.Accept()
		if err != nil {
			select {
			case <-s.done:
				return nil
			default:
			}
			log.L().Warnf(err.Error())
			continue
		}

		go func() {
			s.trackConn(conn, true)
			defer s.trackConn(conn, false)
			addr, err := s.handshake(conn)
			if err != nil {
				log.L().Error("handshake: " + err.Error())
				conn.Close()
				return
			}

			s.handler(conn, addr)
		}()
	}
}

func (s *Server) trackConn(c net.Conn, add bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if add {
		s.activeConn[c] = struct{}{}
	} else {
		delete(s.activeConn, c)
	}
}

func (s *Server) Close() error {
	close(s.done)
	var err error
	if s.lis != nil {
		err = s.lis.Close()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for c := range s.activeConn {
		c.Close()
		delete(s.activeConn, c)
	}
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

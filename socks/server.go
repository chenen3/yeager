// Package socks provides a SOCKS version 5 server implementation.
package socks

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/chenen3/yeager/util"
)

// Server implements the Inbounder interface
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
		return fmt.Errorf("socks5 proxy listen: %s", err)
	}
	s.lis = lis
	log.Printf("socks5 proxy listening %s", s.addr)

	close(s.ready)
	for {
		conn, err := lis.Accept()
		if err != nil {
			return err
		}

		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			defer conn.Close()
			addr, err := s.handshake(conn)
			if err != nil {
				log.Printf("failed to handshake: %s", err)
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

// Shutdown gracefully shuts down the server,
// it works by first closing listener,
// then wait for all connection to close
func (s *Server) Shutdown() error {
	s.cancel()
	var err error
	if s.lis != nil {
		err = s.lis.Close()
	}
	s.wg.Wait()
	return err
}

func (s *Server) handshake(conn net.Conn) (addr string, err error) {
	err = conn.SetDeadline(time.Now().Add(util.HandshakeTimeout))
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

// maxAddrLen is the maximum size of SOCKS address in bytes.
const maxAddrLen = 1 + 1 + 255 + 2

const (
	cmdConnect = 0x01
	// cmdUDPAssociate = 0x03
)

const (
	atypIPv4   = 0x01
	atypDomain = 0x03
	atypIPv6   = 0x04
)

// Refer to https://datatracker.ietf.org/doc/html/rfc1928
func handshake(rw io.ReadWriter) (addr string, err error) {
	buf := make([]byte, maxAddrLen)
	// read VER, NMETHODS, METHODS
	if _, err = io.ReadFull(rw, buf[:2]); err != nil {
		return "", err
	}
	nmethods := buf[1]
	if _, err = io.ReadFull(rw, buf[:nmethods]); err != nil {
		return "", err
	}

	// socks5服务在此仅作为入站代理，使用场景应该是本地内网，无需认证
	// write VER METHOD
	if _, err = rw.Write([]byte{0x05, 0x00}); err != nil {
		return "", err
	}
	// read VER CMD RSV ATYP DST.ADDR DST.PORT
	if _, err = io.ReadFull(rw, buf[:3]); err != nil {
		return "", err
	}

	if cmd := buf[1]; cmd != cmdConnect {
		return "", fmt.Errorf("yet not supported cmd: %x", cmd)
	}

	addr, err = readAddr(rw)
	if err != nil {
		return "", err
	}

	// write VER REP RSV ATYP BND.ADDR BND.PORT
	if _, err = rw.Write([]byte{0x05, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}); err != nil {
		return "", err
	}

	return addr, nil
}

// ReadAddr read SOCKS address from r
// bytes order:
//
//	ATYP BND.ADDR BND.PORT
func readAddr(r io.Reader) (addr string, err error) {
	b := make([]byte, maxAddrLen)
	if _, err = io.ReadFull(r, b[:1]); err != nil {
		return "", err
	}

	var (
		atyp = b[0]
		host string
	)
	switch atyp {
	case atypIPv4:
		if _, err = io.ReadFull(r, b[:net.IPv4len]); err != nil {
			return "", err
		}
		host = net.IPv4(b[0], b[1], b[2], b[3]).String()
	case atypDomain:
		if _, err = io.ReadFull(r, b[:1]); err != nil {
			return "", err
		}
		domainLen := b[0]
		if _, err = io.ReadFull(r, b[:domainLen]); err != nil {
			return "", err
		}
		host = string(b[:domainLen])
	case atypIPv6:
		if _, err = io.ReadFull(r, b[:net.IPv6len]); err != nil {
			return "", err
		}
		ipv6 := make(net.IP, net.IPv6len)
		copy(ipv6, b[:net.IPv6len])
		host = ipv6.String()
	}

	if _, err = io.ReadFull(r, b[:2]); err != nil {
		return "", err
	}

	port := binary.BigEndian.Uint16(b[:2])
	return net.JoinHostPort(host, strconv.Itoa(int(port))), nil
}

package socks

import (
	"context"
	"encoding/binary"
	"errors"
	"expvar"
	"fmt"
	"io"
	"log"
	"net"
	"strconv"
	"sync"
	"time"

	ynet "github.com/chenen3/yeager/net"
	"github.com/chenen3/yeager/tunnel"
)

type Server struct {
	mu          sync.Mutex
	lis         net.Listener
	activeConns map[net.Conn]struct{}
}

// Serve will return a non-nil error unless Close is called.
func (s *Server) Serve(address string, d tunnel.Dialer) error {
	lis, err := net.Listen("tcp", address)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.lis = lis
	s.mu.Unlock()

	for {
		conn, err := lis.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				err = nil
			}
			return err
		}

		// tracking connection in handleConn synchronously will casue unnecessary blocking
		s.trackConn(conn, true)
		go s.handleConn(conn, d)
	}
}

func (s *Server) handleConn(conn net.Conn, d tunnel.Dialer) {
	defer s.trackConn(conn, false)
	defer conn.Close()
	conn.SetReadDeadline(time.Now().Add(ynet.HandshakeTimeout))
	dst, err := handshake(conn)
	if err != nil {
		log.Printf("failed to handshake: %s", err)
		return
	}
	conn.SetReadDeadline(time.Time{})

	ctx, cancel := context.WithTimeout(context.Background(), ynet.DialTimeout)
	defer cancel()
	rwc, err := d.DialContext(ctx, dst)
	if err != nil {
		log.Printf("dial %s error: %s", dst, err)
		return
	}
	defer rwc.Close()

	f := ynet.NewForwarder(conn, rwc)
	// would like to see the goroutine's explicit name while profiling
	go f.FromClient()
	go f.ToClient()
	if err := <-f.C; err != nil {
		log.Printf("forward %s: %s", dst, err)
	}
}

var connCount = expvar.NewInt("socksConnCount")

func (s *Server) trackConn(c net.Conn, add bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.activeConns == nil {
		s.activeConns = make(map[net.Conn]struct{})
	}
	if add {
		s.activeConns[c] = struct{}{}
		connCount.Add(1)
	} else {
		delete(s.activeConns, c)
		connCount.Add(-1)
	}
}

func (s *Server) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var err error
	if s.lis != nil {
		err = s.lis.Close()
	}
	for c := range s.activeConns {
		c.Close()
		delete(s.activeConns, c)
	}
	return err
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

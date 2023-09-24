package proxy

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/chenen3/yeager/flow"
)

// SOCKS server, version 5
type SOCKSServer struct {
	mu         sync.Mutex
	lis        net.Listener
	activeConn map[net.Conn]struct{}
}

// Serve serves connection accepted by lis,
// blocking until the server closes or encounters an unexpected error.
// If dial is nil, the net package's standard dialer is used.
func (s *SOCKSServer) Serve(lis net.Listener, dial dialFunc) error {
	s.mu.Lock()
	s.lis = lis
	s.mu.Unlock()
	if dial == nil {
		dial = defaultDial
	}
	for {
		conn, err := lis.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				err = nil
			}
			return err
		}

		s.trackConn(conn, true)
		go s.handleConn(conn, dial)
	}
}

func (s *SOCKSServer) handleConn(conn net.Conn, dial dialFunc) {
	defer s.trackConn(conn, false)
	defer conn.Close()

	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	addr, err := socksHandshake(conn)
	if err != nil {
		slog.Error("handshake: " + err.Error())
		return
	}
	conn.SetReadDeadline(time.Time{})

	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	stream, err := dial(ctx, addr)
	if err != nil {
		slog.Error(fmt.Sprintf("connect %s: %s", addr, err))
		return
	}
	defer stream.Close()

	err = flow.Relay(conn, stream)
	if err != nil && !canIgnore(err) {
		slog.Error(err.Error())
		return
	}
	slog.Debug("closed "+addr, durationKey, time.Since(start))
}

const durationKey = "dur"

func canIgnore(err error) bool {
	return errors.Is(err, net.ErrClosed) || strings.Contains(err.Error(), "connection reset by peer")
}

func (s *SOCKSServer) trackConn(c net.Conn, add bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.activeConn == nil {
		s.activeConn = make(map[net.Conn]struct{})
	}
	if add {
		s.activeConn[c] = struct{}{}
	} else {
		delete(s.activeConn, c)
	}
}

func (s *SOCKSServer) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var err error
	if s.lis != nil {
		err = s.lis.Close()
	}
	for c := range s.activeConn {
		c.Close()
		delete(s.activeConn, c)
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
func socksHandshake(rw io.ReadWriter) (addr string, err error) {
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
	// VER METHOD
	noAuth := []byte{0x05, 0x00}
	if _, err = rw.Write(noAuth); err != nil {
		return "", err
	}
	// read VER CMD RSV ATYP DST.ADDR DST.PORT
	if _, err = io.ReadFull(rw, buf[:3]); err != nil {
		return "", err
	}

	if cmd := buf[1]; cmd != cmdConnect {
		return "", fmt.Errorf("yet not supported cmd: %x", cmd)
	}

	addr, err = readAddr(rw, buf)
	if err != nil {
		return "", err
	}

	// VER REP RSV ATYP BND.ADDR BND.PORT
	resp := []byte{0x05, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	if _, err = rw.Write(resp); err != nil {
		return "", err
	}

	return addr, nil
}

// read SOCKS address from r
// bytes order:
//
//	ATYP BND.ADDR BND.PORT
func readAddr(r io.Reader, b []byte) (addr string, err error) {
	if len(b) < maxAddrLen {
		return "", errors.New("short buffer")
	}
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
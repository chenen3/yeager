package socks

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strconv"
)

// MaxAddrLen is the maximum size of SOCKS address in bytes.
const MaxAddrLen = 1 + 1 + 255 + 2

const (
	cmdConnect      = 0x01
	cmdUDPAssociate = 0x03
)

const (
	atypIPv4   = 0x01
	atypDomain = 0x03
	atypIPv6   = 0x04
)

// Refer to https://datatracker.ietf.org/doc/html/rfc1928
func handshake(rw io.ReadWriter) (addr string, err error) {
	buf := make([]byte, MaxAddrLen)
	// read VER, NMETHODS, METHODS
	if _, err = io.ReadFull(rw, buf[:2]); err != nil {
		return "", err
	}
	nmethods := buf[1]
	if _, err := io.ReadFull(rw, buf[:nmethods]); err != nil {
		return "", err
	}

	// socks5服务在此仅作为入站代理，使用场景应该是本地内网，无需认证
	// write VER METHOD
	if _, err := rw.Write([]byte{0x05, 0x00}); err != nil {
		return "", err
	}
	// read VER CMD RSV ATYP DST.ADDR DST.PORT
	if _, err := io.ReadFull(rw, buf[:3]); err != nil {
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
//     ATYP BND.ADDR BND.PORT
func readAddr(r io.Reader) (addr string, err error) {
	b := make([]byte, MaxAddrLen)
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

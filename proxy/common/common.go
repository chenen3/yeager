package common

import (
	"encoding/binary"
	"io"
	"net"
	"strconv"
	"time"

	"github.com/chenen3/yeager/util"
)

const (
	DialTimeout       = 4 * time.Second
	HandshakeTimeout  = 5 * time.Second
	MaxConnectionIdle = 5 * time.Minute
)

// MaxAddrLen is the maximum size of SOCKS address in bytes.
const MaxAddrLen = 1 + 1 + 255 + 2

// ReadAddr read SOCKS address from r
// bytes order:
//     ATYP BND.ADDR BND.PORT
func ReadAddr(r io.Reader) (addr string, err error) {
	b := make([]byte, MaxAddrLen)
	if _, err = io.ReadFull(r, b[:1]); err != nil {
		return "", err
	}
	atyp := b[0]

	var host string
	switch atyp {
	case util.AtypIPv4:
		if _, err = io.ReadFull(r, b[:net.IPv4len]); err != nil {
			return "", err
		}
		host = net.IPv4(b[0], b[1], b[2], b[3]).String()
	case util.AtypDomain:
		if _, err = io.ReadFull(r, b[:1]); err != nil {
			return "", err
		}
		domainLen := b[0]
		if _, err = io.ReadFull(r, b[:domainLen]); err != nil {
			return "", err
		}
		host = string(b[:domainLen])
	case util.AtypIPv6:
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

// MarshalAddr encoding addr to bytes in form of SOCKS5 address:
// ATYP BND.ADDR BND.PORT
func MarshalAddr(addr *util.Addr) []byte {
	b := make([]byte, 0, MaxAddrLen)
	b = append(b, byte(addr.Type))

	switch addr.Type {
	case util.AtypIPv4, util.AtypIPv6:
		b = append(b, addr.IP...)
	case util.AtypDomain:
		b = append(b, byte(len(addr.Host)))
		b = append(b, []byte(addr.Host)...)
	}

	var pb [2]byte
	binary.BigEndian.PutUint16(pb[:], uint16(addr.Port))
	b = append(b, pb[:]...)
	return b
}

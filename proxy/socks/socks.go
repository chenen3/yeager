package socks

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"

	"github.com/chenen3/yeager/proxy/common"
	"github.com/chenen3/yeager/util"
)

const (
	ver5   = 0x05 // socks5
	noAuth = 0x00
)

const (
	cmdConnect      = 0x01
	cmdUDPAssociate = 0x03
)

const (
	atypIPv4   = 0x01
	atypDomain = 0x03
	atypIPv6   = 0x04
)

/*
	Each UDP datagram carries a UDP request header with it:
	+----+------+------+----------+----------+----------+
    |RSV | FRAG | ATYP | DST.ADDR | DST.PORT |   DATA   |
    +----+------+------+----------+----------+----------+
    | 2  |  1   |  1   | Variable |    2     | Variable |
    +----+------+------+----------+----------+----------+
*/
type datagram struct {
	dst  *util.Addr
	data []byte
}

func (dg *datagram) Unmarshal(b []byte) error {
	if len(b) <= 5 {
		return errors.New("invalid SOCKS5 UDP data")
	}

	dg.dst = new(util.Addr)
	dg.dst.Type = int(b[3])
	offset := 4 // the offset of DST.ADDR
	var host string
	switch dg.dst.Type {
	case util.AtypIPv4:
		if len(b) < offset+4 {
			return errors.New("invalid SOCKS5 UDP data with bad ipv4")
		}
		hostBs := b[offset : offset+4]
		host = net.IPv4(hostBs[0], hostBs[1], hostBs[2], hostBs[3]).String()
		offset += 4
	case util.AtypDomain:
		domainLen := int(b[offset])
		domainStart := offset + 1
		if len(b) < domainStart+domainLen {
			return errors.New("invalid SOCKS5 UDP data with bad domain")

		}
		host = string(b[domainStart : domainStart+domainLen])
		offset += 1 + domainLen
	default:
		return fmt.Errorf("unsupported address type: %x", dg.dst.Type)
	}

	if len(b) < offset+2 {
		return errors.New("invalid SOCKS5 UDP data with bad port")
	}
	port := binary.BigEndian.Uint16(b[offset : offset+2])

	dst, err := util.ParseAddr("udp", fmt.Sprintf("%s:%d", host, port))
	if err != nil {
		return err
	}
	dg.dst = dst

	offset += 2
	dg.data = b[offset:]
	return nil
}

func (dg *datagram) Marshal() ([]byte, error) {
	var buf bytes.Buffer
	buf.Write([]byte{0x00, 0x00, 0x00})
	buf.Write(common.MarshalAddr(dg.dst))
	buf.Write(dg.data)
	return buf.Bytes(), nil
}

// Refer to https://datatracker.ietf.org/doc/html/rfc1928
func handshake(rw io.ReadWriter) (*util.Addr, error) {
	buf := make([]byte, common.MaxAddrLen)
	// read VER, NMETHODS, METHODS
	if _, err := io.ReadFull(rw, buf[:2]); err != nil {
		return nil, err
	}
	nmethods := buf[1]
	if _, err := io.ReadFull(rw, buf[:nmethods]); err != nil {
		return nil, err
	}

	// socks5服务在此仅作为入站代理，使用场景应该是本地内网，无需认证
	// write VER METHOD
	if _, err := rw.Write([]byte{5, 0}); err != nil {
		return nil, err
	}
	// read VER CMD RSV ATYP DST.ADDR DST.PORT
	if _, err := io.ReadFull(rw, buf[:3]); err != nil {
		return nil, err
	}

	cmd := buf[1]
	var network string
	switch cmd {
	case cmdConnect:
		network = "tcp"
	case cmdUDPAssociate:
		network = "udp"
	default:
		return nil, fmt.Errorf("unsupported cmd: %x", cmd)
	}

	addrS, err := common.ReadAddr(rw)
	if err != nil {
		return nil, err
	}
	addr, err := util.ParseAddr(network, addrS)
	if err != nil {
		return nil, err
	}

	listenAddr, err := util.ParseAddr("tcp", rw.(net.Conn).LocalAddr().String())
	if err != nil {
		return nil, err
	}
	// write VER REP RSV ATYP BND.ADDR BND.PORT
	if _, err = rw.Write(append([]byte{5, 0, 0}, common.MarshalAddr(listenAddr)...)); err != nil {
		return nil, err
	}

	return addr, nil
}

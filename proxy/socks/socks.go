package socks

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"

	"yeager/proxy"
)

const (
	ver5 = 0x05 // socks5
)

type auth byte

const noAuth auth = 0x00

type command byte

const (
	cmdConnect command = 0x01
	cmdUDP     command = 0x03
)

type addressType byte

const (
	atypIPv4   addressType = 0x01
	atypDomain addressType = 0x03
	atypIPv6   addressType = 0x04
)

type reply byte

const (
	success reply = 0x00
)

type AuthRequest struct {
	version  byte
	nMethods byte
	methods  []byte
}

func (r *AuthRequest) Marshal() []byte {
	b := []byte{byte(r.version), r.nMethods}
	b = append(b, r.methods...)
	return b
}

type AuthReply struct {
	version byte
	method  auth
}

func (r *AuthReply) Unmarshal(b []byte) error {
	if len(b) != 2 {
		return fmt.Errorf("invalid input data: %x", b)
	}
	r.version = b[0]
	r.method = auth(b[1])
	return nil
}

type CmdRequest struct {
	version byte
	cmd     command
	addr    *proxy.Address
}

func (r *CmdRequest) Marshal() ([]byte, error) {
	var reverse byte = 0x00
	b := []byte{r.version, byte(r.cmd), reverse}
	switch r.addr.Type {
	case proxy.AddrIPv4:
		b = append(b, byte(atypIPv4))
		b = append(b, r.addr.IP...)
	case proxy.AddrDomainName:
		b = append(b, byte(atypDomain))
		b = append(b, byte(len(r.addr.Host)))
		b = append(b, r.addr.Host...)
	default:
		return nil, fmt.Errorf("unsupported address: %s", r.addr)
	}

	portBs := make([]byte, 2)
	binary.BigEndian.PutUint16(portBs, uint16(r.addr.Port))
	b = append(b, portBs...)
	return b, nil
}

type CmdReply struct {
	version byte
	reply   reply
	addr    *proxy.Address
}

func parseCmdReply(conn net.Conn) (*CmdReply, error) {
	var b [4]byte
	_, err := io.ReadFull(conn, b[:])
	if err != nil {
		return nil, err
	}

	cmdReply := new(CmdReply)
	cmdReply.version, cmdReply.reply = b[0], reply(b[1])
	if cmdReply.version != ver5 {
		return nil, fmt.Errorf("unsupported socks version: %x", cmdReply.version)
	}
	if cmdReply.reply != success {
		return cmdReply, nil
	}

	var host string
	atyp := addressType(b[3])
	switch atyp {
	case atypIPv4:
		var buf [4]byte
		_, err = io.ReadFull(conn, buf[:])
		if err != nil {
			return nil, err
		}
		host = net.IPv4(buf[0], buf[1], buf[2], buf[3]).String()
	case atypDomain:
		var buf [1]byte
		_, err = io.ReadFull(conn, buf[:])
		if err != nil {
			return nil, err
		}
		length := buf[0]

		bs := make([]byte, length)
		_, err = io.ReadFull(conn, bs)
		if err != nil {
			return nil, err
		}
		host = string(bs)
	default:
		return nil, fmt.Errorf("unknown supported address type: %x", atyp)
	}

	var portBuf [2]byte
	_, err = io.ReadFull(conn, portBuf[:])
	if err != nil {
		return nil, err
	}
	port := binary.BigEndian.Uint16(portBuf[:])

	// TODO: network
	addr, err := proxy.ParseHostPort("", host, int(port))
	if err != nil {
		return nil, err
	}

	cmdReply.addr = addr
	return cmdReply, nil
}

/*
	SOCKS5 UDP 与客户端的通信格式(以字节为单位):
	RSV	FRAG	ATYP	DST.ADDR	DST.PORT	DATA
	2	1		1		动态			2			动态
*/
type datagram struct {
	atyp addressType
	dst  *proxy.Address
	data []byte
}

func parseDatagram(b []byte) (*datagram, error) {
	if len(b) <= 5 {
		return nil, errors.New("invalid SOCKS5 UDP data")
	}

	var (
		dg     = &datagram{atyp: addressType(b[3])}
		offset = 4 // the offset of DST.ADDR
		host   string
	)

	switch dg.atyp {
	case atypIPv4:
		if len(b) < offset+4 {
			return nil, errors.New("invalid SOCKS5 UDP data with bad ipv4")
		}
		hostBs := b[offset : offset+4]
		host = net.IPv4(hostBs[0], hostBs[1], hostBs[2], hostBs[3]).String()
		offset += 4
	case atypDomain:
		domainLen := int(b[offset])
		domainStart := offset + 1
		if len(b) < domainStart+domainLen {
			return nil, errors.New("invalid SOCKS5 UDP data with bad domain")

		}
		host = string(b[domainStart : domainStart+domainLen])
		offset += 1 + domainLen
	default:
		return nil, fmt.Errorf("unsupported address type: %x", dg.atyp)
	}

	if len(b) < offset+2 {
		return nil, errors.New("invalid SOCKS5 UDP data with bad port")
	}
	port := binary.BigEndian.Uint16(b[offset : offset+2])

	dst, err := proxy.ParseHostPort("udp", host, int(port))
	if err != nil {
		return nil, err
	}
	dg.dst = dst

	offset += 2
	dg.data = b[offset:]
	return dg, nil
}

func marshalDatagram(dst *proxy.Address, data []byte) []byte {
	var buf bytes.Buffer
	buf.Write([]byte{0x00, 0x00}) // RSV
	buf.WriteByte(0x00)           // FRAG

	switch dst.Type {
	case proxy.AddrIPv4:
		buf.WriteByte(byte(atypIPv4))
		buf.Write(dst.IP)
	case proxy.AddrDomainName:
		buf.WriteByte(byte(atypDomain))
		buf.WriteByte(byte(len(dst.Host)))
		buf.WriteString(dst.Host)
	}

	var portBs [2]byte
	binary.BigEndian.PutUint16(portBs[:], uint16(dst.Port))
	buf.Write(portBs[:])
	buf.Write(data)
	return buf.Bytes()
}

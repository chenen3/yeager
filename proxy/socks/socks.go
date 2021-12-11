package socks

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"

	"github.com/chenen3/yeager/util"
)

const (
	ver5   = 0x05 // socks5
	noAuth = 0x00
)

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

// reply code
const (
	success = 0x00
)

/*
	Refer to https://datatracker.ietf.org/doc/html/rfc1928
	The client connects to the server, and sends a version identifier/method selection message:
	+----+----------+----------+
	|VER | NMETHODS | METHODS  |
	+----+----------+----------+
	| 1  |    1     | 1 to 255 |
	+----+----------+----------+
*/
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
	method  byte
}

func (r *AuthReply) Unmarshal(b []byte) error {
	if len(b) != 2 {
		return fmt.Errorf("invalid input data: %x", b)
	}
	r.version = b[0]
	r.method = b[1]
	return nil
}

/*
	The SOCKS request is formed as follows:
	+----+-----+-------+------+----------+----------+
	|VER | CMD |  RSV  | ATYP | DST.ADDR | DST.PORT |
	+----+-----+-------+------+----------+----------+
	| 1  |  1  | X'00' |  1   | Variable |    2     |
	+----+-----+-------+------+----------+----------+
*/
type CmdRequest struct {
	version byte
	cmd     command
	*util.Address
}

func (r *CmdRequest) Marshal() ([]byte, error) {
	if r.Address == nil {
		return nil, errors.New("empty address")
	}

	var reverse byte = 0x00
	b := []byte{r.version, byte(r.cmd), reverse}
	switch r.Type {
	case util.AddrIPv4:
		b = append(b, byte(atypIPv4))
		b = append(b, r.IP...)
	case util.AddrDomain:
		b = append(b, byte(atypDomain))
		b = append(b, byte(len(r.Host)))
		b = append(b, r.Host...)
	default:
		return nil, fmt.Errorf("unsupported address type: %x", r.Address.Type)
	}

	portBs := make([]byte, 2)
	binary.BigEndian.PutUint16(portBs, uint16(r.Port))
	b = append(b, portBs...)
	return b, nil
}

/*
	The server selects from one of the methods given in METHODS, and
	sends a METHOD selection message:
	+----+--------+
	|VER | METHOD |
	+----+--------+
	| 1  |   1    |
	+----+--------+
*/
type CmdReply struct {
	version byte
	code    byte
	reserve byte
	*util.Address
}

func NewCmdReply(bindAddr string) (*CmdReply, error) {
	addr, err := util.ParseAddress(bindAddr)
	if err != nil {
		return nil, err
	}

	r := &CmdReply{
		version: ver5,
		code:    success,
		reserve: 0x00,
		Address: addr,
	}
	return r, nil
}

// TODO: 从 conn 整块字节读取出来
func parseCmdReply(conn net.Conn) (*CmdReply, error) {
	var b [4]byte
	_, err := io.ReadFull(conn, b[:])
	if err != nil {
		return nil, err
	}

	cmdReply := new(CmdReply)
	cmdReply.version, cmdReply.code = b[0], b[1]
	if cmdReply.version != ver5 {
		return nil, fmt.Errorf("unsupported socks version: %x", cmdReply.version)
	}
	if cmdReply.code != success {
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

	addr, err := util.ParseAddress(net.JoinHostPort(host, strconv.Itoa(int(port))))
	if err != nil {
		return nil, err
	}

	cmdReply.Address = addr
	return cmdReply, nil
}

/*
	The server evaluates the request, and returns a reply formed as follows:
	+----+-----+-------+------+----------+----------+
	|VER | REP |  RSV  | ATYP | BND.ADDR | BND.PORT |
	+----+-----+-------+------+----------+----------+
	| 1  |  1  | X'00' |  1   | Variable |    2     |
	+----+-----+-------+------+----------+----------+
*/
func (rep *CmdReply) Marshal() []byte {
	var buf bytes.Buffer
	buf.Write([]byte{rep.version, rep.code, rep.reserve, byte(rep.Address.Type)})
	switch addressType(rep.Address.Type) {
	case atypIPv4, atypIPv6:
		buf.Write(rep.IP)
	case atypDomain:
		buf.WriteByte(byte(len(rep.Host)))
		buf.Write([]byte(rep.Host))
	}
	var b [2]byte
	binary.BigEndian.PutUint16(b[:], uint16(rep.Port))
	buf.Write(b[:])
	return buf.Bytes()
}

/*
	Each UDP datagram carries a UDP request header with it:
	+----+------+------+----------+----------+----------+
    |RSV | FRAG | ATYP | DST.ADDR | DST.PORT |   DATA   |
    +----+------+------+----------+----------+----------+
    | 2  |  1   |  1   | Variable |    2     | Variable |
    +----+------+------+----------+----------+----------+
*/
type datagram struct {
	dst  *util.Address
	data []byte
}

func (dg *datagram) Unmarshal(b []byte) error {
	if len(b) <= 5 {
		return errors.New("invalid SOCKS5 UDP data")
	}

	dg.dst = new(util.Address)
	dg.dst.Type = util.AddrType(b[3])
	offset := 4 // the offset of DST.ADDR
	var host string
	switch dg.dst.Type {
	case util.AddrIPv4:
		if len(b) < offset+4 {
			return errors.New("invalid SOCKS5 UDP data with bad ipv4")
		}
		hostBs := b[offset : offset+4]
		host = net.IPv4(hostBs[0], hostBs[1], hostBs[2], hostBs[3]).String()
		offset += 4
	case util.AddrDomain:
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

	dst, err := util.ParseAddress(fmt.Sprintf("%s:%d", host, port))
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
	buf.Write([]byte{0x00, 0x00}) // RSV
	buf.WriteByte(0x00)           // FRAG

	switch dg.dst.Type {
	case util.AddrIPv4:
		buf.WriteByte(byte(atypIPv4))
		buf.Write(dg.dst.IP)
	case util.AddrDomain:
		buf.WriteByte(byte(atypDomain))
		buf.WriteByte(byte(len(dg.dst.Host)))
		buf.WriteString(dg.dst.Host)
	}

	var portBs [2]byte
	binary.BigEndian.PutUint16(portBs[:], uint16(dg.dst.Port))
	buf.Write(portBs[:])
	buf.Write(dg.data)
	return buf.Bytes(), nil
}

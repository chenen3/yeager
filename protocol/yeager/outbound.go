package yeager

import (
	"bytes"
	"crypto/tls"
	"encoding/binary"
	"errors"
	"fmt"
	"net"

	"github.com/google/uuid"
	"yeager/protocol"
)

type Client struct {
	conf *ClientConfig
}

func NewClient(config *ClientConfig) *Client {
	return &Client{conf: config}
}

func (c *Client) Dial(dstAddr *protocol.Address) (net.Conn, error) {
	conf := &tls.Config{
		ServerName:         c.conf.Host,
		InsecureSkipVerify: c.conf.InsecureSkipVerify,
	}
	addr := fmt.Sprintf("%s:%d", c.conf.Host, c.conf.Port)
	conn, err := tls.Dial("tcp", addr, conf)
	if err != nil {
		return nil, err
	}
	err = c.handshake(conn, dstAddr)
	if err != nil {
		return nil, err
	}

	return conn, nil
}

const (
	versionBeta     = 0x00
	addressIPv4     = 0x01
	addressDomain   = 0x03
	responseSuccess = 0x00
)

func (c *Client) handshake(conn net.Conn, dstAddr *protocol.Address) error {
	/*
		客户端请求格式，仿照socks5协议(以字节为单位):
		UUID	ATYP	DST.ADDR	DST.PORT
		36		1		动态			2
	*/

	var buf bytes.Buffer
	// write UUID
	sendUUID, err := uuid.Parse(c.conf.UUID)
	if err != nil {
		return err
	}
	buf.WriteString(sendUUID.String())

	// write destination address
	switch dstAddr.Type {
	case protocol.AddrIPv4:
		buf.WriteByte(addressIPv4)
		buf.Write(dstAddr.IP)
	case protocol.AddrDomainName:
		buf.WriteByte(addressDomain)
		buf.WriteByte(byte(len(dstAddr.Host)))
		buf.WriteString(dstAddr.Host)
	default:
		return errors.New("unsupported address type: " + dstAddr.String())
	}
	var b [2]byte
	binary.BigEndian.PutUint16(b[:], uint16(dstAddr.Port))
	buf.Write(b[:])

	_, err = conn.Write(buf.Bytes())
	if err != nil {
		return err
	}

	/*
		服务端回应格式(以字节为单位):
		VER	REP
		1	1
	*/
	var bs [2]byte
	_, err = conn.Read(bs[:])
	if err != nil {
		return err
	}
	rep := bs[1]
	if rep != responseSuccess {
		return fmt.Errorf("fail connecting, received response: %x", rep)
	}

	return nil
}
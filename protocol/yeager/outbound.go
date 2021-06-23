package yeager

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/binary"
	"errors"
	"fmt"
	"net"

	"github.com/google/uuid"
	"yeager/protocol"
	"yeager/util"
)

type Client struct {
	conf *ClientConfig
}

func NewClient(config *ClientConfig) *Client {
	return &Client{conf: config}
}

func (c *Client) DialContext(ctx context.Context, dstAddr *protocol.Address) (net.Conn, error) {
	addr := fmt.Sprintf("%s:%d", c.conf.Host, c.conf.Port)
	d := tls.Dialer{
		Config: &tls.Config{
			ServerName:         c.conf.Host,
			InsecureSkipVerify: c.conf.InsecureSkipVerify,
			ClientSessionCache: tls.NewLRUClientSessionCache(0),
		},
	}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}
	buf, err := c.prepareHandshake(dstAddr)
	if err != nil {
		return nil, err
	}

	return util.NewEarlyWriteConn(conn, buf), nil
}

const (
	addressIPv4   = 0x01
	addressDomain = 0x03
)

// 为了降低握手时延，减少一次RTT，yeager出站代理将在建立tls连接后，第一次发送数据时，
// 附带握手所需的信息（例如目的地址）。因此这里只是构造握手数据，并不是普遍意义上的握手
func (c *Client) prepareHandshake(dstAddr *protocol.Address) (*bytes.Buffer, error) {
	/*
		客户端请求格式，仿照socks5协议(以字节为单位):
		UUID	ATYP	DST.ADDR	DST.PORT
		36		1		动态			2
	*/

	var buf bytes.Buffer
	// write UUID
	sendUUID, err := uuid.Parse(c.conf.UUID)
	if err != nil {
		return nil, err
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
		return nil, errors.New("unsupported address type: " + dstAddr.String())
	}

	var b [2]byte
	binary.BigEndian.PutUint16(b[:], uint16(dstAddr.Port))
	buf.Write(b[:])
	return &buf, nil
}

package yeager

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/binary"
	"errors"
	"net"

	"yeager/config"
	"yeager/proxy"
	"yeager/transport"
	"yeager/transport/grpc"
	ytls "yeager/transport/tls"

	"github.com/google/uuid"
)

type Client struct {
	conf   *config.YeagerClient
	dialer transport.Dialer
}

func NewClient(config *config.YeagerClient) (*Client, error) {
	c := Client{conf: config}
	host, _, err := net.SplitHostPort(config.Address)
	if err != nil {
		return nil, err
	}

	tlsConf := &tls.Config{
		ServerName:         host,
		InsecureSkipVerify: config.Insecure,
		ClientSessionCache: tls.NewLRUClientSessionCache(64),
	}
	switch config.Transport {
	case "tls":
		c.dialer = ytls.NewDialer(tlsConf)
	case "grpc":
		if config.Plaintext {
			c.dialer = grpc.NewDialer(nil)
		} else {
			c.dialer = grpc.NewDialer(tlsConf)
		}
	default:
		return nil, errors.New("unsupported transport: " + config.Transport)
	}

	return &c, nil
}

// 为什么这里实现认证不像socks5/http代理协议那样握手？
// 出站代理与入站代理建立连接时，tcp握手延时加上tls握手延时已经够大了，
// (例如本地机器ping远端VPS是50ms，建立tls连接延时约150ms)，
// 如果为了实现认证而再次握手，正是雪上加霜。
// 因此出站代理将在建立连接后，第一次发送数据时附带凭证，不增加额外延时
func (c *Client) DialContext(ctx context.Context, addr string) (net.Conn, error) {
	conn, err := c.dialer.DialContext(ctx, c.conf.Address)
	if err != nil {
		return nil, err
	}

	cred, err := c.buildCredential(addr)
	if err != nil {
		return nil, err
	}

	newConn := &Conn{
		Conn:       conn,
		earlyWrite: cred,
		maxIdle:    proxy.MaxConnectionIdle,
	}
	return newConn, nil
}

const (
	addressIPv4   = 0x01
	addressDomain = 0x03
)

// buildCredential 构造凭证，包含UUID和目的地址
func (c *Client) buildCredential(addr string) (buf bytes.Buffer, err error) {
	dstAddr, err := proxy.ParseAddress(addr)
	if err != nil {
		return buf, err
	}
	/*
		客户端请求格式，仿照socks5协议(以字节为单位):
		VER UUID ATYP DST.ADDR DST.PORT
		1   36   1    动态     2
	*/

	// keep version number for backward compatibility
	buf.WriteByte(1)

	sendUUID, err := uuid.Parse(c.conf.UUID)
	if err != nil {
		return
	}
	buf.WriteString(sendUUID.String())

	switch dstAddr.Type {
	case proxy.AddrIPv4:
		buf.WriteByte(addressIPv4)
		buf.Write(dstAddr.IP)
	case proxy.AddrDomainName:
		buf.WriteByte(addressDomain)
		buf.WriteByte(byte(len(dstAddr.Host)))
		buf.WriteString(dstAddr.Host)
	default:
		err = errors.New("unsupported address type: " + dstAddr.String())
		return
	}

	var b [2]byte
	binary.BigEndian.PutUint16(b[:], uint16(dstAddr.Port))
	buf.Write(b[:])
	return buf, nil
}

func (c *Client) Close() error {
	return c.dialer.Close()
}

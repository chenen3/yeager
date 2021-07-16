package armin

import (
	"bytes"
	"context"
	gtls "crypto/tls"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"strconv"

	"github.com/google/uuid"
	"yeager/proxy"
	"yeager/transport"
	"yeager/transport/grpc"
	"yeager/transport/tls"
	"yeager/util"
)

type Client struct {
	conf   *ClientConfig
	dialer transport.Dialer
}

func NewClient(config *ClientConfig) (*Client, error) {
	var c Client
	c.conf = config

	tlsConf := &gtls.Config{
		ServerName:         config.TLS.ServerName,
		InsecureSkipVerify: config.TLS.Insecure,
		ClientSessionCache: gtls.NewLRUClientSessionCache(64),
	}
	switch config.Transport {
	case "tls":
		c.dialer = tls.NewDialer(tlsConf)
	case "grpc":
		c.dialer = grpc.NewDialer(tlsConf)
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
func (c *Client) DialContext(ctx context.Context, dst *proxy.Address) (net.Conn, error) {
	addr := net.JoinHostPort(c.conf.Host, strconv.Itoa(c.conf.Port))
	conn, err := c.dialer.DialContext(ctx, addr)
	if err != nil {
		return nil, err
	}

	cred, err := c.buildCredential(dst)
	if err != nil {
		return nil, err
	}

	conn = util.NewMaxIdleConn(conn, proxy.IdleConnTimeout)
	return util.EarlyWriteConn(conn, cred), nil
}

const (
	addressIPv4   = 0x01
	addressDomain = 0x03
)

// buildCredential 构造凭证，包含UUID和目的地址
func (c *Client) buildCredential(dstAddr *proxy.Address) (*bytes.Buffer, error) {
	/*
		客户端请求格式，仿照socks5协议(以字节为单位):
		VER UUID ATYP DST.ADDR DST.PORT
		1   36   1    动态     2
	*/

	var buf bytes.Buffer
	// keep version number for backward compatibility
	buf.WriteByte(1)

	sendUUID, err := uuid.Parse(c.conf.UUID)
	if err != nil {
		return nil, err
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
		return nil, errors.New("unsupported address type: " + dstAddr.String())
	}

	var b [2]byte
	binary.BigEndian.PutUint16(b[:], uint16(dstAddr.Port))
	buf.Write(b[:])
	return &buf, nil
}

func (c *Client) Close() error {
	if closer, ok := c.dialer.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}

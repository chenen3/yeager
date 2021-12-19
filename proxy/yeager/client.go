package yeager

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"os"

	"github.com/chenen3/yeager/config"
	"github.com/chenen3/yeager/proxy/common"
	"github.com/chenen3/yeager/proxy/yeager/transport"
	"github.com/chenen3/yeager/proxy/yeager/transport/grpc"
	"github.com/chenen3/yeager/proxy/yeager/transport/quic"
	"github.com/chenen3/yeager/util"
)

// Client implement interface Outbounder
type Client struct {
	conf   *config.YeagerClient
	dialer transport.Dialer
}

func NewClient(conf *config.YeagerClient) (*Client, error) {
	c := Client{conf: conf}
	switch conf.Transport {
	case config.TransTCP:
		c.dialer = transport.NewTCPDialer()
	case config.TransTLS:
		tc, err := makeClientTLSConfig(conf)
		if err != nil {
			return nil, err
		}
		c.dialer = transport.NewTLSDialer(tc)
	case config.TransGRPC:
		tc, err := makeClientTLSConfig(conf)
		if err != nil {
			return nil, err
		}
		c.dialer = grpc.NewDialer(tc)
	case config.TransQUIC:
		tc, err := makeClientTLSConfig(conf)
		if err != nil {
			return nil, err
		}
		c.dialer = quic.NewDialer(tc)
	default:
		return nil, fmt.Errorf("unsupported transport: %s", conf.Transport)
	}

	return &c, nil
}

// return mutual tls config
func makeClientTLSConfig(conf *config.YeagerClient) (*tls.Config, error) {
	tlsConf := &tls.Config{
		ClientSessionCache: tls.NewLRUClientSessionCache(64),
	}

	if conf.MutualTLS.CertFile != "" {
		cert, err := tls.LoadX509KeyPair(conf.MutualTLS.CertFile, conf.MutualTLS.KeyFile)
		if err != nil {
			return nil, err
		}
		tlsConf.Certificates = []tls.Certificate{cert}
		ca, err := os.ReadFile(conf.MutualTLS.CAFile)
		if err != nil {
			return nil, err
		}
		pool := x509.NewCertPool()
		if ok := pool.AppendCertsFromPEM(ca); !ok {
			return nil, errors.New("failed to parse root certificate")
		}
		tlsConf.RootCAs = pool
	} else if len(conf.MutualTLS.CertPEM) != 0 {
		cert, err := tls.X509KeyPair(conf.MutualTLS.CertPEM, conf.MutualTLS.KeyPEM)
		if err != nil {
			return nil, err
		}
		tlsConf.Certificates = []tls.Certificate{cert}
		pool := x509.NewCertPool()
		if ok := pool.AppendCertsFromPEM(conf.MutualTLS.CAPEM); !ok {
			return nil, errors.New("failed to parse root certificate")
		}
		tlsConf.RootCAs = pool
	} else {
		return nil, errors.New("required client side certificate")
	}

	return tlsConf, nil
}

func (c *Client) DialContext(ctx context.Context, network string, addr string) (net.Conn, error) {
	conn, err := c.dialer.DialContext(ctx, c.conf.Address)
	if err != nil {
		return nil, err
	}

	metadata, err := c.makeMetaData(addr)
	if err != nil {
		return nil, err
	}
	if _, err = conn.Write(metadata.Bytes()); err != nil {
		return nil, err
	}

	return connWithIdleTimeout(conn, common.MaxConnectionIdle), nil
}

const (
	addressIPv4   = 0x01
	addressDomain = 0x03
)

// makeMetaData 构造元数据，包含目的地址
func (c *Client) makeMetaData(addr string) (buf bytes.Buffer, err error) {
	dstAddr, err := util.ParseAddr("tcp", addr)
	if err != nil {
		return buf, err
	}
	/*
		客户端请求格式，仿照socks5协议(以字节为单位):
		VER ATYP DST.ADDR DST.PORT
		1   1    动态     2
	*/

	// keep version number for backward compatibility
	buf.WriteByte(1)

	switch dstAddr.Type {
	case util.AddrIPv4:
		buf.WriteByte(addressIPv4)
		buf.Write(dstAddr.IP)
	case util.AddrDomainName:
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
	if closer, ok := c.dialer.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}

package yeager

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"os"

	"github.com/chenen3/yeager/config"
	"github.com/chenen3/yeager/util"
)

type contextDialer interface {
	DialContext(ctx context.Context, network, addr string) (net.Conn, error)
}

// Client implement the proxy.Outbounder interface
type Client struct {
	conf   *config.YeagerClient
	dialer contextDialer
}

func NewClient(conf *config.YeagerClient) (*Client, error) {
	c := Client{conf: conf}
	switch conf.Transport {
	case config.TransTCP:
		c.dialer = new(net.Dialer)
	case config.TransTLS:
		tc, err := makeClientTLSConfig(conf)
		if err != nil {
			return nil, err
		}
		c.dialer = &tls.Dialer{Config: tc}
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

	if conf.TLS.CertFile != "" {
		cert, err := tls.LoadX509KeyPair(conf.TLS.CertFile, conf.TLS.KeyFile)
		if err != nil {
			return nil, err
		}
		tlsConf.Certificates = []tls.Certificate{cert}
		ca, err := os.ReadFile(conf.TLS.CAFile)
		if err != nil {
			return nil, err
		}
		pool := x509.NewCertPool()
		if ok := pool.AppendCertsFromPEM(ca); !ok {
			return nil, errors.New("failed to parse root certificate")
		}
		tlsConf.RootCAs = pool
	} else if len(conf.TLS.CertPEM) != 0 {
		cert, err := tls.X509KeyPair(conf.TLS.CertPEM, conf.TLS.KeyPEM)
		if err != nil {
			return nil, err
		}
		tlsConf.Certificates = []tls.Certificate{cert}
		pool := x509.NewCertPool()
		if ok := pool.AppendCertsFromPEM(conf.TLS.CAPEM); !ok {
			return nil, errors.New("failed to parse root certificate")
		}
		tlsConf.RootCAs = pool
	} else {
		return nil, errors.New("required client side certificate")
	}

	return tlsConf, nil
}

func (c *Client) DialContext(ctx context.Context, _, addr string) (net.Conn, error) {
	conn, err := c.dialer.DialContext(ctx, "tcp", c.conf.Address)
	if err != nil {
		return nil, err
	}

	header, err := c.makeHeader(addr)
	if err != nil {
		return nil, errors.New("make header: " + err.Error())
	}
	if _, err = conn.Write(header); err != nil {
		return nil, err
	}

	return conn, nil
}

const maxAddrLen = 1 + 1 + 255 + 2

const (
	addrIPv4   = 0x01
	addrDomain = 0x03
	addrIPv6   = 0x04
)

// makeHeader 构造 header，包含目的地址
func (c *Client) makeHeader(addr string) ([]byte, error) {
	dstAddr, err := util.ParseAddr("tcp", addr)
	if err != nil {
		return nil, err
	}

	/*
		客户端请求格式，仿照socks5协议(以字节为单位):
		VER ATYP DST.ADDR DST.PORT
		1   1    动态     2
	*/
	b := make([]byte, 0, 1+maxAddrLen)
	// keep version number for backward compatibility
	b = append(b, 0x00)

	switch dstAddr.Type {
	case util.AddrIPv4:
		b = append(b, addrIPv4)
		b = append(b, dstAddr.IP...)
	case util.AddrDomainName:
		b = append(b, addrDomain)
		b = append(b, byte(len(dstAddr.Host)))
		b = append(b, []byte(dstAddr.Host)...)
	case util.AddrIPv6:
		b = append(b, addrIPv6)
		b = append(b, dstAddr.IP...)
	default:
		return nil, errors.New("unsupported address type: " + dstAddr.String())
	}

	p := make([]byte, 2)
	binary.BigEndian.PutUint16(p, uint16(dstAddr.Port))
	b = append(b, p...)
	return b, nil
}

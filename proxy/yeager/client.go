package yeager

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"errors"
	"github.com/google/uuid"
	"io"
	"net"
	"os"

	"yeager/config"
	"yeager/proxy"
	"yeager/transport"
	"yeager/transport/grpc"
)

type Client struct {
	conf   *config.YeagerClient
	dialer transport.Dialer
}

func NewClient(config *config.YeagerClient) (*Client, error) {
	c := Client{conf: config}
	switch config.Transport {
	case "tls":
		tlsConf, err := makeClientTLSConfig(config)
		if err != nil {
			return nil, err
		}
		c.dialer = &tls.Dialer{Config: tlsConf}
	case "grpc":
		if config.Plaintext {
			c.dialer = grpc.NewDialer(nil)
		} else {
			tlsConf, err := makeClientTLSConfig(config)
			if err != nil {
				return nil, err
			}
			c.dialer = grpc.NewDialer(tlsConf)
		}
	default:
		return nil, errors.New("unsupported transport: " + config.Transport)
	}

	return &c, nil
}

func makeClientTLSConfig(conf *config.YeagerClient) (*tls.Config, error) {
	tlsConf := &tls.Config{
		// ServerName:         host, // FIXME
		InsecureSkipVerify: conf.Insecure,
		ClientSessionCache: tls.NewLRUClientSessionCache(64),
	}
	if conf.RootCAFile != "" {
		// mutual TLS
		cert, err := tls.LoadX509KeyPair(conf.CertFile, conf.KeyFile)
		if err != nil {
			return nil, err
		}
		tlsConf.Certificates = []tls.Certificate{cert}
		ca, err := os.ReadFile(conf.RootCAFile)
		if err != nil {
			return nil, err
		}
		pool := x509.NewCertPool()
		ok := pool.AppendCertsFromPEM(ca)
		if !ok {
			return nil, errors.New("failed to parse root certificate")
		}
		tlsConf.RootCAs = pool
	} else if len(conf.RootCA) != 0 {
		cert, err := tls.X509KeyPair(conf.CertPEM, conf.KeyPEM)
		if err != nil {
			return nil, err
		}
		tlsConf.Certificates = []tls.Certificate{cert}
		pool := x509.NewCertPool()
		ok := pool.AppendCertsFromPEM(conf.RootCA)
		if !ok {
			return nil, errors.New("failed to parse root certificate")
		}
		tlsConf.RootCAs = pool
	}
	return tlsConf, nil
}

func (c *Client) DialContext(ctx context.Context, addr string) (net.Conn, error) {
	conn, err := c.dialer.DialContext(ctx, "tcp", c.conf.Address)
	if err != nil {
		return nil, err
	}

	metadata, err := c.makeMetaData(addr)
	if err != nil {
		return nil, err
	}

	newConn := &Conn{
		Conn:       conn,
		earlyWrite: metadata,
		maxIdle:    proxy.MaxConnectionIdle,
	}
	return newConn, nil
}

const (
	addressIPv4   = 0x01
	addressDomain = 0x03
)

// while using mutual TLS, uuid is ignored,
// for backward compatibility, left it blank
var uuidPlaceholder [36]byte

// makeMetaData 构造元数据，包含目的地址
func (c *Client) makeMetaData(addr string) (buf bytes.Buffer, err error) {
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

	if c.conf.UUID == "" {
		buf.Write(uuidPlaceholder[:])
	} else {
		sendUUID, err := uuid.Parse(c.conf.UUID)
		if err != nil {
			return buf, err
		}
		buf.WriteString(sendUUID.String())
	}

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
	if closer, ok := c.dialer.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}

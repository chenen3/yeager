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

	"github.com/google/uuid"

	"github.com/chenen3/yeager/config"
	"github.com/chenen3/yeager/proxy/common"
	"github.com/chenen3/yeager/transport"
	"github.com/chenen3/yeager/transport/grpc"
	"github.com/chenen3/yeager/transport/quic"
	"github.com/chenen3/yeager/util"
)

type Client struct {
	conf   *config.YeagerClient
	dialer transport.Dialer
}

func NewClient(conf *config.YeagerClient) (*Client, error) {
	var tlsConf *tls.Config
	if conf.Security != config.ClientNoSecurity {
		var err error
		tlsConf, err = makeClientTLSConfig(conf)
		if err != nil {
			return nil, err
		}
	}

	c := Client{conf: conf}
	switch conf.Transport {
	case config.TransTCP:
		if conf.Security == config.ClientNoSecurity {
			c.dialer = new(net.Dialer)
		} else {
			c.dialer = &tls.Dialer{Config: tlsConf}
		}
	case config.TransGRPC:
		c.dialer = grpc.NewDialer(tlsConf, conf.Address)
	case config.TransQUIC:
		c.dialer = quic.NewDialer(tlsConf)
	default:
		return nil, fmt.Errorf("unsupported transport: %s", conf.Transport)
	}

	return &c, nil
}

func makeClientTLSConfig(conf *config.YeagerClient) (*tls.Config, error) {
	tlsConf := &tls.Config{
		ClientSessionCache: tls.NewLRUClientSessionCache(64),
	}

	switch conf.Security {
	case config.ClientTLS:
		tlsConf.InsecureSkipVerify = conf.TLS.Insecure
	case config.ClientTLSMutual:
		if conf.MTLS.CertFile != "" {
			cert, err := tls.LoadX509KeyPair(conf.MTLS.CertFile, conf.MTLS.KeyFile)
			if err != nil {
				return nil, err
			}
			tlsConf.Certificates = []tls.Certificate{cert}
			ca, err := os.ReadFile(conf.MTLS.RootCAFile)
			if err != nil {
				return nil, err
			}
			pool := x509.NewCertPool()
			ok := pool.AppendCertsFromPEM(ca)
			if !ok {
				return nil, errors.New("failed to parse root certificate")
			}
			tlsConf.RootCAs = pool
		} else if len(conf.MTLS.CertPEM) != 0 {
			cert, err := tls.X509KeyPair(conf.MTLS.CertPEM, conf.MTLS.KeyPEM)
			if err != nil {
				return nil, err
			}
			tlsConf.Certificates = []tls.Certificate{cert}
			pool := x509.NewCertPool()
			ok := pool.AppendCertsFromPEM(conf.MTLS.RootCA)
			if !ok {
				return nil, errors.New("failed to parse root certificate")
			}
			tlsConf.RootCAs = pool
		} else {
			return nil, errors.New("required client side certificate")
		}
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
		Conn:     conn,
		metadata: metadata,
		maxIdle:  common.MaxConnectionIdle,
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
	dstAddr, err := util.ParseAddress(addr)
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

	if c.conf.Security == config.ClientTLSMutual {
		// when use mutual authentication, UUID is no longer needed
		buf.Write(uuidPlaceholder[:])
	} else {
		sendUUID, err := uuid.Parse(c.conf.UUID)
		if err != nil {
			return buf, err
		}
		buf.WriteString(sendUUID.String())
	}

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

package quic

import (
	"context"
	"crypto/tls"
	"errors"
	"io"

	ynet "github.com/chenen3/yeager/net"
	"github.com/chenen3/yeager/tunnel"
	"github.com/lucas-clemente/quic-go"
)

type TunnelClient struct {
	pool *connPool
}

func NewTunnelClient(address string, tlsConf *tls.Config, poolSize int) *TunnelClient {
	var c TunnelClient
	dialFunc := func() (quic.Connection, error) {
		qconf := &quic.Config{
			HandshakeIdleTimeout: ynet.HandshakeTimeout,
			MaxIdleTimeout:       ynet.IdleConnTimeout,
			KeepAlivePeriod:      ynet.KeepAlive,
		}
		tlsConf.NextProtos = []string{"quic"}
		return quic.DialAddr(address, tlsConf, qconf)
	}
	c.pool = newConnPool(poolSize, dialFunc)
	return &c
}

func (c *TunnelClient) DialContext(ctx context.Context, dst string) (io.ReadWriteCloser, error) {
	conn, err := c.pool.Get()
	if err != nil {
		return nil, errors.New("dial quic: " + err.Error())
	}

	rawStream, err := conn.OpenStream()
	if err != nil {
		return nil, errors.New("open quic stream: " + err.Error())
	}

	stream := wrapStream(rawStream)
	if err := tunnel.WriteHeader(stream, dst); err != nil {
		stream.Close()
		return nil, err
	}
	return stream, nil
}

func (c *TunnelClient) Close() error {
	if c.pool == nil {
		return nil
	}
	return c.pool.Close()
}

type streamWrapper struct {
	quic.Stream
}

// wrapStream wrap the raw quic.Stream with method Close modified
func wrapStream(raw quic.Stream) *streamWrapper {
	return &streamWrapper{raw}
}

// Close closes read-direction and write-direction of the stream
func (s *streamWrapper) Close() error {
	s.CancelRead(0)
	return s.Stream.Close()
}

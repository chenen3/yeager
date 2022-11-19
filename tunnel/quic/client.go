package quic

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"time"

	"github.com/chenen3/yeager/tunnel"
	"github.com/lucas-clemente/quic-go"
)

type TunnelClient struct {
	pool *Pool
}

func NewTunnelClient(address string, tlsConf *tls.Config, poolSize int) *TunnelClient {
	var c TunnelClient
	dialFunc := func() (quic.Connection, error) {
		qconf := &quic.Config{
			HandshakeIdleTimeout: 5 * time.Second,
			MaxIdleTimeout:       30 * time.Second,
			KeepAlivePeriod:      15 * time.Second,
		}
		tlsConf.NextProtos = []string{"quic"}
		return quic.DialAddr(address, tlsConf, qconf)
	}
	c.pool = NewPool(poolSize, dialFunc)
	return &c
}

func (c *TunnelClient) DialContext(ctx context.Context, addr string) (io.ReadWriteCloser, error) {
	qconn, err := c.pool.Get()
	if err != nil {
		return nil, errors.New("dial quic: " + err.Error())
	}

	rawStream, err := qconn.OpenStream()
	if err != nil {
		return nil, errors.New("open quic stream: " + err.Error())
	}

	stream := wrapStream(rawStream)
	header, err := tunnel.MakeHeader(addr)
	if err != nil {
		stream.Close()
		return nil, err
	}
	_, err = stream.Write(header)
	if err != nil {
		stream.Close()
		return nil, err
	}

	return stream, nil
}

func (c *TunnelClient) Close() error {
	if c.pool != nil {
		return c.pool.Close()
	}
	return nil
}

type stream struct {
	quic.Stream
}

// wrapStream wrap the raw quic.Stream with method Close modified
func wrapStream(raw quic.Stream) *stream {
	return &stream{raw}
}

// Close closes read-direction and write-direction of the stream
func (s *stream) Close() error {
	s.Stream.CancelRead(0)
	return s.Stream.Close()
}

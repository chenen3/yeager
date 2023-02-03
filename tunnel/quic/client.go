package quic

import (
	"context"
	"crypto/tls"
	"errors"
	"expvar"
	"io"
	"sync"

	"github.com/chenen3/yeager/debug"
	ynet "github.com/chenen3/yeager/net"
	"github.com/chenen3/yeager/tunnel"
	"github.com/quic-go/quic-go"
)

var connCount = new(debug.Counter)

func init() {
	expvar.Publish("connquic", connCount)
}

type TunnelClient struct {
	srvAddr string
	tlsConf *tls.Config
	mu      sync.RWMutex
	conns   []quic.Connection
}

const defaultConnNum = 2

func NewTunnelClient(address string, tlsConf *tls.Config, connNum int) *TunnelClient {
	if connNum <= defaultConnNum {
		connNum = defaultConnNum
	}
	tlsConf.NextProtos = []string{"quic"}
	c := &TunnelClient{
		srvAddr: address,
		tlsConf: tlsConf,
		conns:   make([]quic.Connection, connNum),
	}
	connCount.Register(c.countConn)
	return c
}

func isClosed(conn quic.Connection) bool {
	select {
	case <-conn.Context().Done():
		return true
	default:
		return false
	}
}

func (c *TunnelClient) getConn(addr string) (quic.Connection, error) {
	i := len(addr) % len(c.conns)
	c.mu.RLock()
	conn := c.conns[i]
	c.mu.RUnlock()
	if conn != nil && !isClosed(conn) {
		return conn, nil
	}

	conf := &quic.Config{
		HandshakeIdleTimeout: ynet.HandshakeTimeout,
		MaxIdleTimeout:       ynet.IdleTimeout,
		KeepAlivePeriod:      ynet.KeepAlivePeriod,
	}
	newConn, err := quic.DialAddr(c.srvAddr, c.tlsConf, conf)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if conn := c.conns[i]; conn != nil && !isClosed(conn) {
		newConn.CloseWithError(0, "")
		return conn, nil
	}
	c.conns[i] = newConn
	return newConn, nil
}

func (c *TunnelClient) DialContext(ctx context.Context, dst string) (io.ReadWriteCloser, error) {
	conn, err := c.getConn(dst)
	if err != nil {
		return nil, errors.New("dial quic: " + err.Error())
	}

	stream, err := conn.OpenStream()
	if err != nil {
		return nil, errors.New("open quic stream: " + err.Error())
	}

	sw := wrapStream(stream)
	if err := tunnel.WriteHeader(sw, dst); err != nil {
		sw.Close()
		return nil, err
	}
	return sw, nil
}

func (c *TunnelClient) countConn() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	var i int
	for _, conn := range c.conns {
		if conn != nil && !isClosed(conn) {
			i++
		}
	}
	return i
}

func (c *TunnelClient) Close() error {
	var err error
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, conn := range c.conns {
		if conn == nil {
			continue
		}
		e := conn.CloseWithError(0, "")
		if e != nil {
			err = e
		}
	}
	return err
}

type streamWrapper struct {
	quic.Stream
}

// wrapStream wrap quic.Stream with method Close modified
func wrapStream(raw quic.Stream) *streamWrapper {
	return &streamWrapper{raw}
}

// Close closes the read and write directions of the stream.
// Since quic.Stream.Close() does not close reads and writes by convention,
// future reads will block forever.
func (s *streamWrapper) Close() error {
	s.CancelRead(ynet.StreamNoError)
	return s.Stream.Close()
}

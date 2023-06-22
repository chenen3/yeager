package quic

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"sync"
	"time"

	"github.com/chenen3/yeager/debug"
	ynet "github.com/chenen3/yeager/net"
	"github.com/chenen3/yeager/tunnel"
	"github.com/quic-go/quic-go"
)

type TunnelClient struct {
	addr   string
	conf   *tls.Config
	mu     sync.RWMutex // guards conns
	conns  map[string]quic.Connection
	ticker *time.Ticker
}

func NewTunnelClient(addr string, tlsConf *tls.Config) *TunnelClient {
	tlsConf.NextProtos = []string{"quic"}
	host, _, _ := net.SplitHostPort(addr)
	tlsConf.ServerName = host
	c := &TunnelClient{
		addr:   addr,
		conf:   tlsConf,
		ticker: time.NewTicker(time.Minute),
		conns:  make(map[string]quic.Connection),
	}
	go c.sweep()
	return c
}

// sweep periodically clears connections closed due to idle timeout
func (c *TunnelClient) sweep() {
	for range c.ticker.C {
		c.mu.Lock()
		for key, conn := range c.conns {
			if isClosed(conn) {
				delete(c.conns, key)
				debug.Printf("clear idle conn %s", key)
			}
		}
		c.mu.Unlock()
	}
}

func isClosed(conn quic.Connection) bool {
	select {
	case <-conn.Context().Done():
		return true
	default:
		return false
	}
}

func (c *TunnelClient) getConn(ctx context.Context, key string) (quic.Connection, error) {
	c.mu.RLock()
	conn, ok := c.conns[key]
	c.mu.RUnlock()
	if ok && !isClosed(conn) {
		return conn, nil
	}

	newconn, err := quic.DialAddr(ctx, c.addr, c.conf, &quic.Config{
		HandshakeIdleTimeout: ynet.HandshakeTimeout,
		MaxIdleTimeout:       idleTimeout,
	})
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if cc, ok := c.conns[key]; ok && !isClosed(cc) {
		// other goroutine has set up new connection
		newconn.CloseWithError(0, "")
		return cc, nil
	}

	c.conns[key] = newconn
	return newconn, nil
}

func (c *TunnelClient) DialContext(ctx context.Context, dst string) (io.ReadWriteCloser, error) {
	conn, err := c.getConn(ctx, dst)
	if err != nil {
		return nil, errors.New("quic dial: " + err.Error())
	}
	stream, err := conn.OpenStreamSync(ctx)
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

func (c *TunnelClient) ConnNum() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.conns)
}

func (c *TunnelClient) Close() error {
	if c.ticker != nil {
		c.ticker.Stop()
	}
	var err error
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, conn := range c.conns {
		e := conn.CloseWithError(0, "")
		if e != nil && err == nil {
			err = e
		}
	}
	c.conns = nil
	return err
}

type streamWrapper struct {
	quic.Stream
}

// wrap the stream with modified method Close
func wrapStream(stream quic.Stream) *streamWrapper {
	return &streamWrapper{
		Stream: stream,
	}
}

func (s *streamWrapper) Close() error {
	// stop receiving on this stream since quic.Stream.Close() does not handle this
	s.CancelRead(0)
	return s.Stream.Close()
}

package quic

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"log"
	"net"
	"sync"
	"time"

	"github.com/chenen3/yeager/debug"
	ynet "github.com/chenen3/yeager/net"
	"github.com/chenen3/yeager/tunnel"
	"github.com/quic-go/quic-go"
)

type TunnelClient struct {
	conf   TunnelClientConfig
	mu     sync.RWMutex // guards conns
	conns  map[string]quic.Connection
	ticker *time.Ticker
}

type TunnelClientConfig struct {
	Target      string
	TLSConfig   *tls.Config
	watchPeriod time.Duration
	idleTimeout time.Duration
}

func NewTunnelClient(conf TunnelClientConfig) *TunnelClient {
	if conf.TLSConfig == nil {
		panic("TLS config required")
	}
	conf.TLSConfig.NextProtos = []string{"quic"}
	if conf.watchPeriod == 0 {
		conf.watchPeriod = time.Minute
	}
	if conf.idleTimeout == 0 {
		conf.idleTimeout = ynet.IdleTimeout
	}
	c := &TunnelClient{
		conf:   conf,
		ticker: time.NewTicker(conf.watchPeriod),
		conns:  make(map[string]quic.Connection),
	}
	go c.watch()
	return c
}

// watch periodically clears connections closed due to idle timeout
func (c *TunnelClient) watch() {
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

	newconn, err := quic.DialAddrContext(ctx, c.conf.Target, c.conf.TLSConfig, &quic.Config{
		HandshakeIdleTimeout: ynet.HandshakeTimeout,
		MaxIdleTimeout:       c.conf.idleTimeout,
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

// openStream opens a new stream, reconnect if necessary.
func (c *TunnelClient) openStream(ctx context.Context, key string) (quic.Stream, error) {
	conn, err := c.getConn(ctx, key)
	if err != nil {
		return nil, err
	}
	stream, err := conn.OpenStream()
	if err == nil {
		return stream, nil
	}

	if ne, ok := err.(net.Error); ok && ne.Temporary() {
		// reaching the peer's stream limit
		return nil, ne
	}
	log.Printf("streaming error: %s, reconnecting...", err)
	conn.CloseWithError(0, "")
	rc, re := c.getConn(ctx, key)
	if re != nil {
		return nil, re
	}
	return rc.OpenStream()
}

func (c *TunnelClient) DialContext(ctx context.Context, dst string) (io.ReadWriteCloser, error) {
	stream, err := c.openStream(ctx, dst)
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
	s.CancelRead(ynet.StreamNoError)
	return s.Stream.Close()
}

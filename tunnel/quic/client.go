package quic

import (
	"context"
	"crypto/tls"
	"errors"
	"expvar"
	"io"
	"sync"
	"time"

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
	conf   TunnelClientConfig
	mu     sync.RWMutex // guards conns
	conns  map[string]quic.Connection
	ticker *time.Ticker
}

type TunnelClientConfig struct {
	Target      string
	TLSConfig   *tls.Config
	WatchPeriod time.Duration // default to 1 minute
	IdleTimeout time.Duration // default to 2 minutes
}

func NewTunnelClient(conf TunnelClientConfig) *TunnelClient {
	if conf.TLSConfig == nil {
		panic("TLS config required")
	}
	conf.TLSConfig.NextProtos = []string{"quic"}
	if conf.WatchPeriod == 0 {
		conf.WatchPeriod = time.Minute
	}
	if conf.IdleTimeout == 0 {
		conf.IdleTimeout = ynet.IdleTimeout
	}

	c := &TunnelClient{
		conf:   conf,
		ticker: time.NewTicker(conf.WatchPeriod),
		conns:  make(map[string]quic.Connection),
	}
	go c.watch()
	connCount.Register(c.countConn)
	return c
}

func (c *TunnelClient) watch() {
	for range c.ticker.C {
		c.mu.Lock()
		c.clearConnectionLocked()
		c.mu.Unlock()
	}
}

// clearConnectionLocked clears connections that have been closed due to idle timeouts.
func (c *TunnelClient) clearConnectionLocked() {
	if len(c.conns) == 0 {
		return
	}

	n := len(c.conns)
	for key, conn := range c.conns {
		if isClosed(conn) {
			delete(c.conns, key)
		}
	}
	if len(c.conns) < n {
		debug.Logf("clear %d connections", n-len(c.conns))
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

	conf := &quic.Config{
		HandshakeIdleTimeout: ynet.HandshakeTimeout,
		MaxIdleTimeout:       c.conf.IdleTimeout,
		// it seems MaxIdleTimeout does not work when keep-alive enabled
		// KeepAlivePeriod: ynet.KeepAlivePeriod,
	}
	newconn, err := quic.DialAddrContext(ctx, c.conf.Target, c.conf.TLSConfig, conf)
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
		return nil, errors.New("dial quic: " + err.Error())
	}

	stream, err := conn.OpenStream()
	if err != nil {
		return nil, errors.New("open quic stream: " + err.Error())
	}

	sw := wrapStream(stream, nil)
	if err := tunnel.WriteHeader(sw, dst); err != nil {
		sw.Close()
		return nil, err
	}
	return sw, nil
}

func (c *TunnelClient) countConn() int {
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
	onClose func()
	once    sync.Once
}

func wrapStream(stream quic.Stream, onClose func()) *streamWrapper {
	return &streamWrapper{
		Stream:  stream,
		onClose: onClose,
	}
}

func (s *streamWrapper) Close() error {
	if s.onClose != nil {
		s.once.Do(s.onClose)
	}
	// stop receiving on this stream since quic.Stream.Close() does not handle this
	s.CancelRead(ynet.StreamNoError)
	return s.Stream.Close()
}

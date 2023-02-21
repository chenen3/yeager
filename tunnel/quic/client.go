package quic

import (
	"context"
	"crypto/tls"
	"errors"
	"expvar"
	"io"
	"sync"
	"sync/atomic"
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
	conf        TunnelClientConfig
	mu          sync.RWMutex // guards conns
	conns       []quic.Connection
	streamCount int32
	ticker      *time.Ticker
}

type TunnelClientConfig struct {
	Target            string
	TLSConfig         *tls.Config
	WatchPeriod       time.Duration // default to 1 minute
	IdleTimeout       time.Duration // default to 2 minutes
	MaxStreamsPerConn int           // default to 100
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
	if conf.MaxStreamsPerConn <= 0 {
		// MaxIncomingStreams of quic.Config default to 100
		conf.MaxStreamsPerConn = 100
	}

	c := &TunnelClient{
		conf:   conf,
		ticker: time.NewTicker(conf.WatchPeriod),
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

	live := make([]quic.Connection, 0, len(c.conns))
	for _, conn := range c.conns {
		if !isClosed(conn) {
			live = append(live, conn)
		}
	}
	if len(live) < len(c.conns) {
		c.conns = live
		debug.Logf("scale down to %d connection", len(live))
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

func (c *TunnelClient) getConn(ctx context.Context) (quic.Connection, error) {
	i := int(atomic.LoadInt32(&c.streamCount)) / c.conf.MaxStreamsPerConn
	c.mu.RLock()
	if i < len(c.conns) && !isClosed(c.conns[i]) {
		// everything is fine
		conn := c.conns[i]
		c.mu.RUnlock()
		return conn, nil
	}
	c.mu.RUnlock()

	conf := &quic.Config{
		HandshakeIdleTimeout: ynet.HandshakeTimeout,
		MaxIdleTimeout:       c.conf.IdleTimeout,
		// it seems MaxIdleTimeout does not work when keep-alive enabled
		// KeepAlivePeriod: ynet.KeepAlivePeriod,
	}
	conn, err := quic.DialAddrContext(ctx, c.conf.Target, c.conf.TLSConfig, conf)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if i >= len(c.conns) {
		// still need more connections
		c.conns = append(c.conns, conn)
		debug.Logf("scale up to %d connection", len(c.conns))
		return conn, nil
	}

	if !isClosed(c.conns[i]) {
		// other goroutine has set up new connection
		conn.CloseWithError(0, "")
		return c.conns[i], nil
	}

	c.conns[i] = conn
	return conn, nil
}

func (c *TunnelClient) DialContext(ctx context.Context, dst string) (io.ReadWriteCloser, error) {
	conn, err := c.getConn(ctx)
	if err != nil {
		return nil, errors.New("dial quic: " + err.Error())
	}

	stream, err := conn.OpenStream()
	if err != nil {
		return nil, errors.New("open quic stream: " + err.Error())
	}

	atomic.AddInt32(&c.streamCount, 1)
	sw := wrapStream(stream, func() { atomic.AddInt32(&c.streamCount, -1) })
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

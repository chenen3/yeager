package quic

import (
	"context"
	"crypto/tls"
	"errors"
	"expvar"
	"io"
	"log"
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

type TunnelClientConfig struct {
	Target            string
	TLSConfig         *tls.Config
	WatchPeriod       time.Duration // default to 1 minute
	IdleTimeout       time.Duration // default to 2 minutes
	MaxStreamsPerConn int           // default to 100
}

func (cc *TunnelClientConfig) tidy() TunnelClientConfig {
	if cc.TLSConfig == nil {
		// tidy() will be called on initialization, do not return an error
		panic("TLS config required")
	}
	cc.TLSConfig.NextProtos = []string{"quic"}
	if cc.WatchPeriod == 0 {
		cc.WatchPeriod = time.Minute
	}
	if cc.IdleTimeout == 0 {
		cc.IdleTimeout = ynet.IdleTimeout
	}
	if cc.MaxStreamsPerConn <= 0 {
		// MaxIncomingStreams of quic.Config default to 100
		cc.MaxStreamsPerConn = 100
	}
	return *cc
}

type TunnelClient struct {
	conf        TunnelClientConfig
	mu          sync.RWMutex // guards conns
	conns       []quic.Connection
	streamCount int32
	done        chan struct{}
}

func NewTunnelClient(conf TunnelClientConfig) *TunnelClient {
	c := &TunnelClient{
		conf: conf.tidy(),
		done: make(chan struct{}),
	}
	go c.watch()
	connCount.Register(c.countConn)
	return c
}

func (c *TunnelClient) watch() {
	tick := time.NewTicker(c.conf.WatchPeriod)
	for {
		select {
		case <-c.done:
			tick.Stop()
		case <-tick.C:
			c.mu.Lock()
			c.clearConnectionLocked()
			c.mu.Unlock()
		}
	}
}

// clearConnectionLocked clears connections that have been closed due to idle timeouts.
func (c *TunnelClient) clearConnectionLocked() {
	if len(c.conns) == 0 {
		return
	}

	live := make([]quic.Connection, 0, len(c.conns))
	for _, conn := range c.conns {
		if conn != nil && !isClosed(conn) {
			live = append(live, conn)
		}
	}
	if len(live) < len(c.conns) {
		c.conns = live
		if debug.Enabled() {
			log.Printf("scale down to %d connection", len(live))
		}
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

func (c *TunnelClient) getConn() (quic.Connection, error) {
	i := int(atomic.LoadInt32(&c.streamCount)) / c.conf.MaxStreamsPerConn
	c.mu.Lock()
	if i < len(c.conns) {
		conn := c.conns[i]
		if !isClosed(conn) {
			c.mu.Unlock()
			return conn, nil
		}
		if i == 0 {
			c.conns = nil
		} else if i == len(c.conns)-1 {
			c.conns = c.conns[:i-1]
		} else {
			c.conns = append(c.conns[:i], c.conns[i+1:]...)
		}
	}
	c.mu.Unlock()

	conf := &quic.Config{
		HandshakeIdleTimeout: ynet.HandshakeTimeout,
		MaxIdleTimeout:       c.conf.IdleTimeout,
		// it seems MaxIdleTimeout does not work when keep-alive enabled
		// KeepAlivePeriod: ynet.KeepAlivePeriod,
	}
	newConn, err := quic.DialAddr(c.conf.Target, c.conf.TLSConfig, conf)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	c.conns = append(c.conns, newConn)
	if debug.Enabled() {
		log.Printf("scale up to %d connection", len(c.conns))
	}
	c.mu.Unlock()
	return newConn, nil
}

func (c *TunnelClient) DialContext(ctx context.Context, dst string) (io.ReadWriteCloser, error) {
	conn, err := c.getConn()
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
	close(c.done)
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
	c.conns = nil
	return err
}

type streamWrapper struct {
	quic.Stream
	onClose func()
	once    sync.Once
}

// wrapStream wrap quic.Stream with method Close modified
func wrapStream(stream quic.Stream, onClose func()) *streamWrapper {
	return &streamWrapper{
		Stream:  stream,
		onClose: onClose,
	}
}

// Close closes the read and write directions of the stream.
// Since quic.Stream.Close() does not close reads and writes by convention,
// future reads will block forever.
func (s *streamWrapper) Close() error {
	if s.onClose != nil {
		s.once.Do(s.onClose)
	}
	s.CancelRead(ynet.StreamNoError)
	return s.Stream.Close()
}

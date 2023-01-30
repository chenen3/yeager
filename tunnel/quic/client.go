package quic

import (
	"context"
	"crypto/tls"
	"errors"
	"expvar"
	"io"
	"log"
	"sync"
	"time"

	"github.com/chenen3/yeager/debug"
	ynet "github.com/chenen3/yeager/net"
	"github.com/chenen3/yeager/tunnel"
	"github.com/lucas-clemente/quic-go"
)

var connCount = new(debug.Counter)

func init() {
	expvar.Publish("connquic", connCount)
}

type TunnelClient struct {
	srvAddr string
	tlsConf *tls.Config
	mu      sync.RWMutex
	conns   map[string]quic.Connection
	done    chan struct{}
}

func NewTunnelClient(address string, tlsConf *tls.Config) *TunnelClient {
	tlsConf.NextProtos = []string{"quic"}
	c := &TunnelClient{
		srvAddr: address,
		tlsConf: tlsConf,
		conns:   make(map[string]quic.Connection),
		done:    make(chan struct{}),
	}
	go c.watch()
	connCount.Register(c.Len)
	return c
}

const watchPeriod = 2 * time.Minute

func (c *TunnelClient) watch() {
	tick := time.NewTicker(watchPeriod)
	for {
		select {
		case <-c.done:
			tick.Stop()
			return
		case <-tick.C:
			c.mu.Lock()
			for key, conn := range c.conns {
				if isClosed(conn) {
					delete(c.conns, key)
					if debug.Enabled() {
						log.Printf("clear idle timeout connection: %s", key)
					}
				}
			}
			c.mu.Unlock()
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

func (c *TunnelClient) getConn(addr string) (quic.Connection, error) {
	c.mu.RLock()
	conn, ok := c.conns[addr]
	c.mu.RUnlock()
	if ok && !isClosed(conn) {
		return conn, nil
	}

	conf := &quic.Config{
		HandshakeIdleTimeout: ynet.HandshakeTimeout,
		MaxIdleTimeout:       ynet.IdleTimeout,
		// experiement shows that once keepalive is enabled,
		// the quic connection won't be closed even if MaxIdleTimeout is exceeded
		// KeepAlivePeriod: ynet.KeepAlivePeriod,
	}
	newConn, err := quic.DialAddr(c.srvAddr, c.tlsConf, conf)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if conn, ok := c.conns[addr]; ok && !isClosed(conn) {
		newConn.CloseWithError(0, "")
		return conn, nil
	}
	c.conns[addr] = newConn
	return newConn, nil
}

func (c *TunnelClient) DialContext(ctx context.Context, dst string) (io.ReadWriteCloser, error) {
	conn, err := c.getConn(dst)
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

func (c *TunnelClient) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.conns)
}

func (c *TunnelClient) Close() error {
	close(c.done)
	var err error
	c.mu.Lock()
	defer c.mu.Unlock()
	for key, conn := range c.conns {
		e := conn.CloseWithError(quic.ApplicationErrorCode(quic.NoError), "")
		if e != nil {
			err = e
		}
		delete(c.conns, key)
	}
	return err
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
	s.CancelRead(ynet.ErrCodeCancelRead)
	return s.Stream.Close()
}

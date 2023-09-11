package quic

import (
	"context"
	"crypto/tls"
	"io"
	"net"
	"sync"
	"time"

	"github.com/quic-go/quic-go"
)

type TunnelClient struct {
	addr  string
	conf  *tls.Config
	mu    sync.Mutex
	conns []quic.Connection
}

func NewTunnelClient(addr string, tlsConf *tls.Config) *TunnelClient {
	tlsConf.NextProtos = []string{"quic"}
	host, _, _ := net.SplitHostPort(addr)
	tlsConf.ServerName = host
	return &TunnelClient{
		addr: addr,
		conf: tlsConf,
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

// TODO: if the number of outgoing streams exceeds MaxIncomingStreams,
// issue new connection (see previous implementation)
func (c *TunnelClient) getConn(ctx context.Context, key string) (quic.Connection, error) {
	c.mu.Lock()
	for i, conn := range c.conns {
		if !isClosed(conn) {
			c.conns = c.conns[i:]
			c.mu.Unlock()
			return conn, nil
		}
	}
	c.mu.Unlock()

	conn, err := quic.DialAddr(ctx, c.addr, c.conf, &quic.Config{
		HandshakeIdleTimeout: 5 * time.Second,
		MaxIdleTimeout:       idleTimeout,
		KeepAlivePeriod:      15 * time.Second,
	})
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	c.conns = append(c.conns, conn)
	c.mu.Unlock()
	return conn, nil
}

func (c *TunnelClient) connect(ctx context.Context) (quic.Stream, error) {
	c.mu.Lock()
	for _, conn := range c.conns {
		if isClosed(conn) {
			continue
		}
		stream, err := conn.OpenStream()
		if err != nil {
			// refer to unexported streamOpenErr in quic-go
			if te, ok := err.(interface{ Temporary() bool }); ok && te.Temporary() {
				// reach max streams limit
				continue
			}
			c.mu.Unlock()
			return nil, err
		}
		c.mu.Unlock()
		return stream, nil
	}
	c.mu.Unlock()

	conn, err := quic.DialAddr(ctx, c.addr, c.conf, &quic.Config{
		HandshakeIdleTimeout: 5 * time.Second,
		MaxIdleTimeout:       idleTimeout,
		KeepAlivePeriod:      15 * time.Second,
	})
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	active := []quic.Connection{conn}
	for _, conn := range c.conns {
		if !isClosed(conn) {
			active = append(active, conn)
		}
	}
	c.conns = active
	c.mu.Unlock()
	return conn.OpenStream()
}

func (c *TunnelClient) DialContext(ctx context.Context, dst string) (io.ReadWriteCloser, error) {
	stream, err := c.connect(ctx)
	if err != nil {
		return nil, err
	}
	stream = wrapStream(stream)
	m := metadata{Hostport: dst}
	if _, err := m.WriteTo(stream); err != nil {
		stream.Close()
		return nil, err
	}
	return stream, nil
}

func (c *TunnelClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	var err error
	for _, conn := range c.conns {
		e := conn.CloseWithError(0, "")
		if e != nil && err == nil {
			err = e
		}
	}
	return err
}

type streamWrapper struct {
	quic.Stream
}

// makes stream's method Close close both read and write
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

type metadata struct {
	Hostport string
}

// [length, payload...]
func (m *metadata) WriteTo(w io.Writer) (int64, error) {
	size := len(m.Hostport)
	buf := make([]byte, 0, size+1)
	buf = append(buf, byte(size))
	buf = append(buf, []byte(m.Hostport)...)
	n, err := w.Write(buf)
	return int64(n), err
}

func (m *metadata) ReadFrom(r io.Reader) (int64, error) {
	var b [1]byte
	ns, err := io.ReadFull(r, b[:])
	if err != nil {
		return int64(ns), err
	}

	size := int(b[0])
	buf := make([]byte, size)
	n, err := io.ReadFull(r, buf)
	if err != nil {
		return int64(n), err
	}
	m.Hostport = string(buf)
	return int64(ns + n), nil
}

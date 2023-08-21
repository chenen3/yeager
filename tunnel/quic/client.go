package quic

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"sync"
	"time"

	ynet "github.com/chenen3/yeager/net"
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
		HandshakeIdleTimeout: ynet.HandshakeTimeout,
		MaxIdleTimeout:       idleTimeout,
		KeepAlivePeriod:      15 * time.Second,
		EnableDatagrams:      true,
	})
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	c.conns = append(c.conns, conn)
	c.mu.Unlock()
	return conn, nil
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
	m := metadata{Hostport: dst}
	if _, err := m.WriteTo(stream); err != nil {
		return nil, err
	}
	return wrapStream(stream), nil
}

func (c *TunnelClient) ConnNum() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.conns)
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

// wrap the stream with modified Close method
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
func (m *metadata) Bytes() []byte {
	size := len(m.Hostport)
	bs := make([]byte, 0, size+1)
	bs = append(bs, byte(size))
	bs = append(bs, []byte(m.Hostport)...)
	return bs
}

func (m *metadata) WriteTo(w io.Writer) (int64, error) {
	n, err := w.Write(m.Bytes())
	return int64(n), err
}

func (m *metadata) ReadFrom(r io.Reader) (int64, error) {
	var b [1]byte
	ns, err := io.ReadFull(r, b[:])
	if err != nil {
		return int64(ns), err
	}

	size := int(b[0])
	bs := make([]byte, size)
	n, err := io.ReadFull(r, bs)
	if err != nil {
		return int64(n), err
	}
	m.Hostport = string(bs)
	return int64(ns + n), nil
}

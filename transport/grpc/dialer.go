package grpc

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"sync"
	"time"

	"github.com/chenen3/yeager/transport"
	"github.com/chenen3/yeager/transport/grpc/pb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/metadata"
)

type streamDialer struct {
	addr  string
	cfg   *tls.Config
	mu    sync.Mutex
	conns []*grpc.ClientConn
}

var _ transport.StreamDialer = (*streamDialer)(nil)

// NewStreamDialer creates a transport.StreamDialer that
// connects to the specified gRPC server address.
// The caller should call Close when finished, to close the underlying connections.
func NewStreamDialer(addr string, cfg *tls.Config) *streamDialer {
	return &streamDialer{
		addr: addr,
		cfg:  cfg,
	}
}

const keepaliveInterval = 15 * time.Second

// getConn tends to use existing client connections, dialing new ones if necessary.
// To mitigate the website fingerprinting via multiplexing in HTTP/2,
// fewer connections will be better.
func (d *streamDialer) getConn(ctx context.Context) (*grpc.ClientConn, error) {
	d.mu.Lock()
	for i, cc := range d.conns {
		if s := cc.GetState(); s != connectivity.Shutdown && s != connectivity.TransientFailure {
			if i > 0 {
				// clear dead conn
				d.conns = d.conns[i:]
			}
			d.mu.Unlock()
			return cc, nil
		}
		cc.Close()
	}
	d.mu.Unlock()

	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(credentials.NewTLS(d.cfg)),
		grpc.WithConnectParams(grpc.ConnectParams{
			Backoff: backoff.Config{
				BaseDelay:  1.0 * time.Second,
				Multiplier: 1.5,
				Jitter:     0.2,
				MaxDelay:   5 * time.Second,
			},
			MinConnectTimeout: 5 * time.Second,
		}),
		grpc.WithBlock(), // blocking dial facilitates clear logic while creating stream
		grpc.WithIdleTimeout(idleTimeout),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:    keepaliveInterval,
			Timeout: 2 * time.Second,
		}),
	}
	conn, err := grpc.DialContext(ctx, d.addr, opts...)
	if err != nil {
		return nil, err
	}

	d.mu.Lock()
	d.conns = append(d.conns, conn)
	d.mu.Unlock()
	return conn, nil
}

const targetKey = "target"

func (d *streamDialer) Dial(ctx context.Context, target string) (transport.Stream, error) {
	conn, err := d.getConn(ctx)
	if err != nil {
		return nil, errors.New("grpc conenct: " + err.Error())
	}

	client := pb.NewTunnelClient(conn)
	// this context controls the lifetime of the stream, do not use short-lived contexts
	sctx, cancel := context.WithCancel(context.Background())
	sctx = metadata.NewOutgoingContext(sctx, metadata.Pairs(targetKey, target))
	stream, err := client.Stream(sctx)
	if err != nil {
		cancel()
		conn.Close()
		return nil, err
	}
	return &clientStream{stream: stream, onClose: cancel}, nil
}

func (c *streamDialer) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, cc := range c.conns {
		cc.Close()
	}
	return nil
}

// clientStream implements transport.Stream.
// It also implements io.WriterTo and io.ReaderFrom as optimizations
// so io.Copy can avoid allocating unnecessary buffers.
type clientStream struct {
	stream  pb.Tunnel_StreamClient
	onClose func()
	buf     []byte
}

var _ transport.Stream = (*clientStream)(nil)
var _ io.WriterTo = (*clientStream)(nil)
var _ io.ReaderFrom = (*clientStream)(nil)

func (c *clientStream) Read(b []byte) (n int, err error) {
	if len(c.buf) == 0 {
		m, err := c.stream.Recv()
		if err != nil {
			return 0, err
		}
		c.buf = m.Data
	}
	n = copy(b, c.buf)
	c.buf = c.buf[n:]
	return n, nil
}

func (c *clientStream) Write(b []byte) (n int, err error) {
	if err = c.stream.Send(&pb.Message{Data: b}); err != nil {
		return 0, err
	}
	return len(b), nil
}

func (c *clientStream) Close() error {
	if c.onClose != nil {
		c.onClose()
	}
	return nil
}

func (c *clientStream) CloseWrite() error {
	return c.stream.CloseSend()
}

// WriteTo uses buffer received from grpc stream, instead of allocating a new one
func (c *clientStream) WriteTo(w io.Writer) (written int64, err error) {
	for {
		msg, er := c.stream.Recv()
		if msg != nil && len(msg.Data) > 0 {
			nr := len(msg.Data)
			nw, ew := w.Write(msg.Data)
			if nw < 0 || nr < nw {
				nw = 0
				if ew == nil {
					ew = errors.New("invalid write result")
				}
			}
			written += int64(nw)
			if ew != nil {
				err = ew
				break
			}
			if nr != nw {
				err = io.ErrShortWrite
				break
			}
		}
		if er != nil {
			if er != io.EOF {
				err = er
			}
			break
		}
	}
	return written, err
}

var bufPool = sync.Pool{
	New: func() any {
		buf := make([]byte, 16*1024)
		return &buf
	},
}

func (c *clientStream) ReadFrom(r io.Reader) (n int64, err error) {
	buf := bufPool.Get().(*[]byte)
	for {
		nr, er := r.Read(*buf)
		if nr > 0 {
			ew := c.stream.Send(&pb.Message{Data: (*buf)[:nr]})
			if ew != nil {
				err = ew
				break
			}
			n += int64(nr)
		}
		if er != nil {
			if er != io.EOF {
				err = er
			}
			break
		}
	}
	bufPool.Put(buf)
	return n, err
}

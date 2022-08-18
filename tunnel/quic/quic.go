package quic

import (
	"context"
	"crypto/tls"
	"errors"
	"log"
	"net"
	"time"

	"github.com/lucas-clemente/quic-go"

	"github.com/chenen3/yeager/util"
)

type dialer struct {
	tlsConf *tls.Config
	pool    *Pool
}

// NewDialer return a QUIC dialer that implements the tunnel.Dialer interface
func NewDialer(tlsConf *tls.Config, addr string, poolSize int) *dialer {
	d := &dialer{tlsConf: tlsConf}
	dialFunc := func() (quic.Connection, error) {
		qconf := &quic.Config{
			HandshakeIdleTimeout: 5 * time.Second,
			MaxIdleTimeout:       30 * time.Second,
			KeepAlivePeriod:      15 * time.Second,
		}
		d.tlsConf.NextProtos = []string{"quic"}
		return quic.DialAddr(addr, d.tlsConf, qconf)
	}
	d.pool = NewPool(poolSize, dialFunc)
	return d
}

func (d *dialer) DialContext(ctx context.Context) (net.Conn, error) {
	qconn, err := d.pool.Get()
	if err != nil {
		return nil, errors.New("dial quic: " + err.Error())
	}

	stream, err := qconn.OpenStream()
	if err != nil {
		return nil, errors.New("open quic stream: " + err.Error())
	}

	conn := &streamConn{
		Stream:     stream,
		localAddr:  qconn.LocalAddr(),
		remoteAddr: qconn.RemoteAddr(),
	}
	return conn, nil
}

func (d *dialer) Close() error {
	if d.pool != nil {
		return d.pool.Close()
	}
	return nil
}

// listener implements the net.Listener interface
type listener struct {
	ctx       context.Context
	cancelCtx context.CancelFunc
	lis       quic.Listener
	connCh    chan net.Conn
}

func (l *listener) acceptLoop() {
	for {
		qconn, err := l.lis.Accept(l.ctx)
		if err != nil {
			if !errors.Is(err, context.Canceled) {
				log.Printf("accept quic connection: %s", err)
			}
			break
		}

		go func() {
			defer qconn.CloseWithError(quic.ApplicationErrorCode(quic.NoError), "")
			for {
				stream, err := qconn.AcceptStream(context.Background())
				if err != nil {
					if !errors.Is(err, net.ErrClosed) {
						log.Printf("accept quic stream: %s", err)
					}
					break
				}

				select {
				case <-stream.Context().Done():
					continue
				default:
				}
				l.connCh <- &streamConn{
					Stream:     stream,
					localAddr:  qconn.LocalAddr(),
					remoteAddr: qconn.RemoteAddr(),
				}
			}
		}()
	}
}

func (l *listener) Accept() (net.Conn, error) {
	select {
	case <-l.ctx.Done():
		return nil, net.ErrClosed
	case c := <-l.connCh:
		return c, nil
	}
}

func (l *listener) Close() error {
	l.cancelCtx()
	return l.lis.Close()
}

func (l *listener) Addr() net.Addr {
	return l.lis.Addr()
}

// Listen creates a QUIC server listening on a given address
func Listen(addr string, tlsConf *tls.Config) (net.Listener, error) {
	if tlsConf == nil {
		return nil, errors.New("tls config required")
	}

	tlsConf.NextProtos = []string{"quic"}
	lis, err := quic.ListenAddr(addr, tlsConf, &quic.Config{MaxIdleTimeout: util.MaxConnectionIdle})
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	l := &listener{
		ctx:       ctx,
		cancelCtx: cancel,
		lis:       lis,
		connCh:    make(chan net.Conn, 32),
	}
	go l.acceptLoop()
	return l, nil
}

// streamConn wraps quic.Stream and implements net.Conn interface
type streamConn struct {
	quic.Stream
	localAddr  net.Addr
	remoteAddr net.Addr
}

func (sc *streamConn) LocalAddr() net.Addr {
	return sc.localAddr
}

func (sc *streamConn) RemoteAddr() net.Addr {
	return sc.remoteAddr
}

func (sc *streamConn) Close() error {
	// here need to close both read-direction and write-direction of the stream,
	// while quic.Stream.Close() only closes the write-direction
	sc.CancelRead(0)
	return sc.Stream.Close()
}

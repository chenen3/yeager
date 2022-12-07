package quic

import (
	"errors"
	"sync"
	"sync/atomic"

	"github.com/lucas-clemente/quic-go"
)

const defaultSize = 1

type connPool struct {
	size     int
	i        uint32
	mu       sync.RWMutex
	conns    []quic.Connection
	dialFunc func() (quic.Connection, error)
	done     chan struct{}
}

func newConnPool(size int, dialFunc func() (quic.Connection, error)) *connPool {
	if size <= 0 {
		size = defaultSize
	}

	return &connPool{
		size:     size,
		conns:    make([]quic.Connection, size),
		dialFunc: dialFunc,
		done:     make(chan struct{}),
	}
}

func isValid(conn quic.Connection) bool {
	if conn == nil {
		return false
	}

	select {
	case <-conn.Context().Done():
		return false
	default:
		return true
	}
}

func (p *connPool) Get() (quic.Connection, error) {
	select {
	case <-p.done:
		return nil, errors.New("pool closed")
	default:
	}

	i := 0
	if p.size > 1 {
		i = int(atomic.AddUint32(&p.i, 1)) % p.size
	}
	p.mu.RLock()
	conn := p.conns[i]
	p.mu.RUnlock()
	if isValid(conn) {
		return conn, nil
	}

	// release resource of the dead connection
	if conn != nil {
		e := quic.ApplicationErrorCode(quic.ApplicationErrorErrorCode)
		conn.CloseWithError(e, "dead connection")
	}

	qc, err := p.dialFunc()
	if err != nil {
		return nil, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if isValid(p.conns[i]) {
		qc.CloseWithError(0, "")
		return p.conns[i], nil
	}
	p.conns[i] = qc
	return qc, nil
}

func (p *connPool) Close() error {
	close(p.done)
	var err error
	p.mu.RLock()
	defer p.mu.RUnlock()
	for _, c := range p.conns {
		if c == nil {
			continue
		}
		e := c.CloseWithError(quic.ApplicationErrorCode(quic.NoError), "")
		if e != nil {
			err = e
		}
	}
	return err
}

package quic

import (
	"errors"
	"log"
	"sync/atomic"

	"github.com/lucas-clemente/quic-go"
)

const defaultSize = 2

type connPool struct {
	size      int
	i         uint32
	conns     []quic.Connection
	factory   connFactoryFunc
	reconnect chan int
	done      chan struct{}
}

type connFactoryFunc func() (quic.Connection, error)

func newConnPool(size int, factory connFactoryFunc) *connPool {
	if size <= 0 {
		size = defaultSize
	}

	p := &connPool{
		size:      size,
		conns:     make([]quic.Connection, size),
		factory:   factory,
		reconnect: make(chan int, size*2),
		done:      make(chan struct{}),
	}
	go p.reconnectLoop()
	for i := 0; i < size; i++ {
		c, err := p.factory()
		if err != nil {
			log.Printf("connect quic: %s", err)
			continue
		}
		p.conns[i] = c
	}
	return p
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

func (p *connPool) reconnectLoop() {
	for {
		select {
		case <-p.done:
			return
		case i := <-p.reconnect:
			if isValid(p.conns[i]) {
				continue
			}
			c, err := p.factory()
			if err != nil {
				log.Printf("connect quic: %s", err)
				continue
			}
			p.conns[i] = c
		}
	}
}

func (p *connPool) Get() (quic.Connection, error) {
	i := int(atomic.AddUint32(&p.i, 1)) % p.size
	conn := p.conns[i]
	if !isValid(conn) {
		go func() {
			p.reconnect <- i
		}()
		return nil, errors.New("invalid connection")
	}
	return conn, nil
}

func (p *connPool) Close() error {
	close(p.done)
	var err error
	for _, c := range p.conns {
		e := c.CloseWithError(quic.ApplicationErrorCode(quic.NoError), "connection closed")
		if e != nil {
			err = e
		}
	}
	return err
}

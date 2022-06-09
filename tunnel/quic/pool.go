package quic

import (
	"errors"
	"log"
	"sync/atomic"
	"time"

	"github.com/lucas-clemente/quic-go"
)

const defaultSize = 2

// QUIC connection pool
type Pool struct {
	size         int
	i            uint32
	conns        []quic.Connection
	dialFunc     func() (quic.Connection, error)
	reconnecting chan int
	reconnected  chan quic.Connection
	done         chan struct{}
}

// NewPool creates a QUIC connection pool
func NewPool(size int, dialFunc func() (quic.Connection, error)) *Pool {
	if size <= 0 {
		size = defaultSize
	}

	p := &Pool{
		size:         size,
		conns:        make([]quic.Connection, size),
		dialFunc:     dialFunc,
		reconnecting: make(chan int, size*2),
		reconnected:  make(chan quic.Connection, size*4),
		done:         make(chan struct{}),
	}
	go p.reconnectLoop()
	for i := 0; i < size; i++ {
		c, err := p.dialFunc()
		if err != nil {
			log.Printf("dial quic: %s", err)
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

func (p *Pool) reconnectLoop() {
	for {
		select {
		case <-p.done:
			return
		case i := <-p.reconnecting:
			if isValid(p.conns[i]) {
				go func() {
					p.reconnected <- p.conns[i]
				}()
				continue
			}
			if p.conns[i] != nil {
				e := quic.ApplicationErrorCode(quic.ApplicationErrorErrorCode)
				// release resource
				p.conns[i].CloseWithError(e, "dead connection")
			}
			c, err := p.dialFunc()
			if err != nil {
				log.Printf("reconnect quic: %s", err)
				continue
			}
			p.conns[i] = c
			go func() {
				p.reconnected <- c
			}()
		}
	}
}

func (p *Pool) Get() (quic.Connection, error) {
	i := int(atomic.AddUint32(&p.i, 1)) % p.size
	conn := p.conns[i]
	if isValid(conn) {
		return conn, nil
	}

	go func() {
		p.reconnecting <- i
	}()
	t := time.NewTimer(time.Second)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			return nil, errors.New("dead connection")
		case conn := <-p.reconnected:
			return conn, nil
		}
	}
}

func (p *Pool) Close() error {
	close(p.done)
	var err error
	for _, c := range p.conns {
		e := c.CloseWithError(quic.ApplicationErrorCode(quic.NoError), "")
		if e != nil {
			err = e
		}
	}
	return err
}

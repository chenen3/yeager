package quic

import (
	"errors"
	"log"
	"sync/atomic"
	"time"

	"github.com/lucas-clemente/quic-go"
)

const defaultSize = 2

type connPool struct {
	size      int
	i         uint32
	conns     []quic.Connection
	dialFunc  func() (quic.Connection, error)
	reconnect chan int
	done      chan struct{}
}

func NewConnPool(size int, dialFunc func() (quic.Connection, error)) *connPool {
	if size <= 0 {
		size = defaultSize
	}

	p := &connPool{
		size:      size,
		conns:     make([]quic.Connection, size),
		dialFunc:  dialFunc,
		reconnect: make(chan int, size*2),
		done:      make(chan struct{}),
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

func (p *connPool) notifyReconnect(i int) {
	p.reconnect <- i
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
		}
	}
}

func (p *connPool) Get() (quic.Connection, error) {
	i := int(atomic.AddUint32(&p.i, 1)) % p.size
	conn := p.conns[i]
	if isValid(conn) {
		return conn, nil
	}

	go p.notifyReconnect(i)
	t := time.NewTimer(time.Second)
	defer t.Stop()
	// retry to find a valid connection
	for {
		select {
		case <-t.C:
			return nil, errors.New("all dead")
		default:
			for _, conn := range p.conns {
				if isValid(conn) {
					return conn, nil
				}
			}
		}
	}
}

func (p *connPool) Close() error {
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

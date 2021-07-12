package util

import (
	"context"
	"net"
	"sync"
	"time"

	"yeager/log"
)

// ConnPool implement connection pool, automatically create and cache connection,
// MUST call Init() firstly to initialize itself
type ConnPool struct {
	ch            chan *persistConn
	done          chan struct{}
	once          sync.Once
	retryInterval time.Duration

	Capacity    int
	IdleTimeout time.Duration // how long the cached connection could remain idle
	DialContext func(ctx context.Context) (net.Conn, error)
}

func (p *ConnPool) Init() {
	defaultCapacity := 10
	capacity := p.Capacity
	if capacity <= 0 {
		capacity = defaultCapacity
	}
	p.ch = make(chan *persistConn, capacity)
	p.done = make(chan struct{})
	p.retryInterval = 30 * time.Second
	go p.createConn()
}

// Get return cached or newly-created connection
func (p *ConnPool) Get(ctx context.Context) (net.Conn, error) {
	select {
	case pc := <-p.ch:
		if pc.expire {
			break
		}
		pc.idleTimer.Stop()
		return pc.Conn, nil
	default:
	}

	return p.DialContext(ctx)
}

func (p *ConnPool) createConn() {
	for {
		select {
		case <-p.done:
			close(p.ch)
			return
		default:
		}

		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		conn, err := p.DialContext(ctx)
		cancel()
		if err != nil {
			log.Error(err)
			time.Sleep(p.retryInterval)
			continue
		}

		pc := new(persistConn)
		pc.Conn = conn
		if p.IdleTimeout > 0 {
			pc.idleTimer = time.AfterFunc(p.IdleTimeout, func() {
				pc.Close()
			})
		}

		select {
		case <-p.done:
			pc.Close()
			close(p.ch)
			return
		case p.ch <- pc:
		}
	}
}

func (p *ConnPool) Close() error {
	close(p.done)
	for pc := range p.ch {
		pc.Close()
	}
	return nil
}

type persistConn struct {
	net.Conn
	idleTimer *time.Timer
	expire    bool
}

func (pc *persistConn) Close() error {
	if pc.idleTimer != nil {
		pc.idleTimer.Stop()
	}
	pc.expire = true
	return pc.Conn.Close()
}

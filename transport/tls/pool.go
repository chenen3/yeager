package tls

import (
	"context"
	"net"
	"sync"
	"time"

	"yeager/log"
)

// ConnPool implement connection pool, automatically create and cache connection,
type ConnPool struct {
	ch            chan *persistConn
	done          chan struct{}
	once          sync.Once
	retryInterval time.Duration

	cap         int
	idleTimeout time.Duration
	DialContext func(ctx context.Context, addr string) (net.Conn, error)
}

func NewPool(capacity int, idleTimeout time.Duration,
	dialFunc func(ctx context.Context, addr string) (net.Conn, error)) *ConnPool {
	if capacity <= 0 {
		capacity = 10
	}
	return &ConnPool{
		cap:           capacity,
		idleTimeout:   idleTimeout,
		DialContext:   dialFunc,
		ch:            make(chan *persistConn, capacity),
		done:          make(chan struct{}),
		retryInterval: 30 * time.Second,
	}
}

// Get return cached or newly-created connection
func (p *ConnPool) Get(ctx context.Context, addr string) (net.Conn, error) {
	p.once.Do(func() {
		go p.createConn(addr)
	})

	select {
	case pc := <-p.ch:
		if pc.expire {
			break
		}
		pc.idleTimer.Stop()
		return pc.Conn, nil
	default:
	}

	return p.DialContext(ctx, addr)
}

func (p *ConnPool) createConn(addr string) {
	for {
		select {
		case <-p.done:
			close(p.ch)
			return
		default:
		}

		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		conn, err := p.DialContext(ctx, addr)
		cancel()
		if err != nil {
			log.Warn(err)
			time.Sleep(p.retryInterval)
			continue
		}

		pc := new(persistConn)
		pc.Conn = conn
		if p.idleTimeout > 0 {
			pc.idleTimer = time.AfterFunc(p.idleTimeout, func() {
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

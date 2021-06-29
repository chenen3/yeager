package util

import (
	"context"
	"net"
	"time"

	"yeager/log"
)

// ConnPool implement connection pool, dials connection automatically
// and keep it for future usage. MUST call Init() firstly to initialize itself
type ConnPool struct {
	ch            chan *persistConn
	done          chan struct{}
	retryInterval time.Duration

	Capacity    int
	IdleTimeout time.Duration
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
	go p.makeConn()
}

// Get try to get an available connection from cache, if failed, dial one
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

func (p *ConnPool) makeConn() {
	for {
		select {
		case <-p.done:
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
			return
		case p.ch <- pc:
		}
	}
}

func (p *ConnPool) Close() error {
	close(p.done)
	close(p.ch)
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

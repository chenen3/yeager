package grpc

import (
	"errors"
	"sync"
	"sync/atomic"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
)

// 如何预估连接池大小：
//
//	每个 gRPC connection 可能使用多个 HTTP/2 连接，连接的数量基于该服务器解析的IP数量，
//	每个连接通常限制 100 个并发的 stream (可以用 MaxConcurrentStreams 修改)
//	假设目标服务器只有 1 个IP，gRPC connection 使用 1 条连接，平均每条连接处理 50 个并发请求，
//	需要的 connection 数量是 ceil(并发请求数 / 50)
//	例如预估有 100 个并发请求，需要 ceil(100 / 50) == 2 个 connection，连接池大小为 2
const defaultSize = 1

type connPool struct {
	size     int
	i        uint32
	mu       sync.RWMutex // guard conns
	conns    []*grpc.ClientConn
	dialFunc func() (*grpc.ClientConn, error)
	done     chan struct{}
}

func newConnPool(size int, dialFunc func() (*grpc.ClientConn, error)) *connPool {
	if size <= 0 {
		size = defaultSize
	}

	return &connPool{
		size:     size,
		conns:    make([]*grpc.ClientConn, size),
		dialFunc: dialFunc,
		done:     make(chan struct{}),
	}
}

func (p *connPool) Get() (*grpc.ClientConn, error) {
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
	if conn != nil {
		if conn.GetState() == connectivity.Shutdown {
			return nil, errors.New("dead connection")
		}
		return conn, nil
	}

	cc, err := p.dialFunc()
	if err != nil {
		return nil, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.conns[i] != nil {
		cc.Close()
		return p.conns[i], nil
	}
	p.conns[i] = cc
	return cc, nil
}

func (p *connPool) Close() error {
	close(p.done)
	var err error
	for _, c := range p.conns {
		if c == nil {
			continue
		}
		if e := c.Close(); e != nil && err == nil {
			err = e
		}
	}
	return err
}

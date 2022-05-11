package grpc

import (
	"errors"
	"log"
	"sync/atomic"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
)

// 如何预估连接池大小：
//   每个 gRPC connection 可能使用多个 HTTP/2 连接，连接的数量基于该服务器解析的IP数量，
//   每个连接通常限制 100 个并发的 stream (可以用 MaxConcurrentStreams 修改)
//   假设目标服务器只有 1 个IP，gRPC connection 使用 1 条连接，平均每条连接处理 50 个并发请求，
//   需要的 connection 数量是 ceil(并发请求数 / 50)
//   例如预估有 100 个并发请求，需要 ceil(100 / 50) == 2 个 connection，连接池大小为 2
const defaultPoolSize = 2

// gRPC 连接池
type connPool struct {
	size    int
	i       uint32
	conns   []*grpc.ClientConn
	factory connFactoryFunc
	done    chan struct{}
}

type connFactoryFunc func() (*grpc.ClientConn, error)

func newConnPool(size int, factory connFactoryFunc) *connPool {
	if size <= 0 {
		size = defaultPoolSize
	}

	p := &connPool{
		size:    size,
		conns:   make([]*grpc.ClientConn, size),
		factory: factory,
		done:    make(chan struct{}),
	}

	for i := 0; i < size; i++ {
		c, err := factory()
		if err != nil {
			log.Printf("connect grpc: %s", err)
			continue
		}
		p.conns[i] = c
	}
	return p
}

func isValid(c *grpc.ClientConn) bool {
	return c != nil && c.GetState() != connectivity.Shutdown
}

func (p *connPool) Get() (*grpc.ClientConn, error) {
	i := int(atomic.AddUint32(&p.i, 1)) % p.size
	conn := p.conns[i]
	if !isValid(conn) {
		return nil, errors.New("invalid grpc connection")
	}
	return conn, nil
}

func (p *connPool) Close() error {
	close(p.done)
	var err error
	for _, c := range p.conns {
		if e := c.Close(); e != nil {
			// still need to close other connections, do not return here
			err = e
			log.Printf("close grpc connection: %s", e)
		}
	}
	return err
}

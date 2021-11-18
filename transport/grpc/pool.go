package grpc

import (
	"go.uber.org/atomic"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
)

// 如何预估连接池大小：
//   每个 gRPC channel 可能使用多个 HTTP/2 连接，连接的数量基于该服务器解析的IP数量，
//   每个连接通常限制 100 个并发的 stream (可以用 MaxConcurrentStreams 修改)
//   假设目标服务器只有 1 个IP，gRPC channel 使用 1 条连接，平均每条连接处理 50 个并发请求，
//   需要的 channel 数量是 ceil(并发请求数 / 50)
//   例如预估有 100 个并发请求，需要 ceil(100 / 50) == 2 个 channel，连接池大小为 2
const defaultPoolSize = 2

// gRPC 连接池，实现多个 channel 循环发送请求
type channelPool struct {
	size       int
	index      atomic.Int32
	channels   []*grpc.ClientConn
	factory    channelFactoryFunc
	chRecreate chan int // inside this chan is channels' element index
	done       chan struct{}
}

type channelFactoryFunc func() (*grpc.ClientConn, error)

func newChannelPool(size int, factory channelFactoryFunc) *channelPool {
	if size <= 0 {
		size = defaultPoolSize
	}

	p := &channelPool{
		size:       size,
		channels:   make([]*grpc.ClientConn, size),
		factory:    factory,
		chRecreate: make(chan int, size),
		done:       make(chan struct{}),
	}
	go p.recreateLoop()

	for i := 0; i < size; i++ {
		c, err := factory()
		if err != nil {
			zap.S().Errorf("create grpc channel: %s", err)
			continue
		}
		p.channels[i] = c
	}
	return p
}

func isShutdown(c *grpc.ClientConn) bool {
	return c == nil || c.GetState() == connectivity.Shutdown
}

func (p *channelPool) recreateLoop() {
	for {
		select {
		case <-p.done:
			return
		case i := <-p.chRecreate:
			if !isShutdown(p.channels[i]) {
				// gRPC channel will recover automatically
				continue
			}
			channel, err := p.factory()
			if err != nil {
				zap.S().Errorf("recreate grpc channel: %s", err)
				continue
			}
			p.channels[i] = channel
		}
	}
}

func (p *channelPool) get() *grpc.ClientConn {
	i := int(p.index.Inc()) % p.size
	channel := p.channels[i]
	if isShutdown(channel) {
		p.chRecreate <- i
		return nil
	}
	return channel
}

func (p *channelPool) Close() error {
	close(p.done)
	var err error
	for _, c := range p.channels {
		if e := c.Close(); e != nil {
			// also need to close other channels, so do not return
			err = e
			zap.S().Warnf("close grpc channel: %s", e)
		}
	}
	return err
}

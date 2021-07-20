package armin

import (
	"bytes"
	"io"
	"net"
	"sync"
	"time"

	"yeager/log"
	"yeager/proxy"
)

// Conn implement interface proxy.Conn
type Conn struct {
	net.Conn
	dstAddr     *proxy.Address
	earlyWrite  bytes.Buffer
	once        sync.Once
	idleTimeout time.Duration
}

func (c *Conn) DstAddr() *proxy.Address {
	return c.dstAddr
}

func (c *Conn) Read(b []byte) (n int, err error) {
	c.once.Do(func() {
		err = c.Conn.SetDeadline(time.Now().Add(c.idleTimeout))
		if err != nil {
			log.Error(err)
		}
	})

	n, err = c.Conn.Read(b)
	if c.idleTimeout > 0 && n > 0 && err == nil {
		err = c.Conn.SetDeadline(time.Now().Add(c.idleTimeout))
		if err != nil {
			return 0, err
		}
	}
	return n, err
}

func (c *Conn) Write(p []byte) (n int, err error) {
	c.once.Do(func() {
		err = c.Conn.SetDeadline(time.Now().Add(c.idleTimeout))
		if err != nil {
			log.Error(err)
		}
	})

	if c.earlyWrite.Len() > 0 {
		_, err = c.earlyWrite.WriteTo(c.Conn)
		if err != nil {
			return 0, err
		}
	}

	n, err = c.Conn.Write(p)
	if c.idleTimeout > 0 && n > 0 && err == nil {
		err = c.Conn.SetDeadline(time.Now().Add(c.idleTimeout))
		if err != nil {
			return 0, err
		}
	}
	return n, err
}

type writerOnly struct {
	io.Writer
}

func (c *Conn) ReadFrom(r io.Reader) (n int64, err error) {
	if rf, ok := c.Conn.(io.ReaderFrom); ok {
		return rf.ReadFrom(r)
	}
	// use wrapper to hide existing c.ReadFrom from io.Copy, avoid infinite loop
	return io.Copy(writerOnly{c}, r)
}

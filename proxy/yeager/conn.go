package yeager

import (
	"bytes"
	"net"
	"sync"
	"time"

	"github.com/chenen3/yeager/log"
)

type Conn struct {
	net.Conn
	metadata bytes.Buffer
	once     sync.Once
	maxIdle  time.Duration
}

func (c *Conn) Read(b []byte) (n int, err error) {
	if c.maxIdle > 0 {
		c.once.Do(func() {
			err = c.Conn.SetDeadline(time.Now().Add(c.maxIdle))
			if err != nil {
				log.L().Error(err)
			}
		})
	}

	n, err = c.Conn.Read(b)
	if c.maxIdle > 0 && n > 0 && err == nil {
		err = c.Conn.SetDeadline(time.Now().Add(c.maxIdle))
		if err != nil {
			return 0, err
		}
	}
	return n, err
}

func (c *Conn) Write(p []byte) (n int, err error) {
	if c.maxIdle > 0 {
		c.once.Do(func() {
			err = c.Conn.SetDeadline(time.Now().Add(c.maxIdle))
			if err != nil {
				log.L().Error(err)
			}
		})
	}

	if c.metadata.Len() > 0 {
		_, err = c.metadata.WriteTo(c.Conn)
		if err != nil {
			return 0, err
		}
	}

	n, err = c.Conn.Write(p)
	if c.maxIdle > 0 && n > 0 && err == nil {
		err = c.Conn.SetDeadline(time.Now().Add(c.maxIdle))
		if err != nil {
			return 0, err
		}
	}
	return n, err
}

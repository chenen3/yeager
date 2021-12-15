package yeager

import (
	"net"
	"time"
)

// Conn wraps net.Conn, implement idle timeout by repeatedly
// extending the deadline after successful Read or Write calls.
type Conn struct {
	net.Conn
	maxIdle time.Duration
}

// given zero value for maxIdle means no idle timeout
func connWithIdleTimeout(conn net.Conn, maxIdle time.Duration) *Conn {
	c := &Conn{
		Conn:    conn,
		maxIdle: maxIdle,
	}
	c.extendDeadline()
	return c
}

func (c *Conn) Read(b []byte) (n int, err error) {
	n, err = c.Conn.Read(b)
	if n > 0 && err == nil {
		err = c.extendDeadline()
	}
	return n, err
}

func (c *Conn) Write(p []byte) (n int, err error) {
	n, err = c.Conn.Write(p)
	if n > 0 && err == nil {
		err = c.extendDeadline()
	}
	return n, err
}

func (c *Conn) extendDeadline() error {
	if c.maxIdle <= 0 {
		return nil
	}
	return c.Conn.SetDeadline(time.Now().Add(c.maxIdle))
}

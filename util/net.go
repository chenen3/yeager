package util

import (
	"net"
	"time"
)

func ChoosePort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	ln.Close()
	return ln.Addr().(*net.TCPAddr).Port, nil
}

type connWithIdle struct {
	net.Conn
	idleTimeout time.Duration
}

// net connection with idle timeout
func ConnWithIdleTimeout(c net.Conn, t time.Duration) *connWithIdle {
	return &connWithIdle{
		Conn:        c,
		idleTimeout: t,
	}
}

func (c *connWithIdle) Read(b []byte) (n int, err error) {
	n, err = c.Conn.Read(b)
	// According to net.Conn document:
	// An idle timeout can be implemented by repeatedly extending
	// the deadline after successful Read or Write calls.
	if c.idleTimeout > 0 && n > 0 && err == nil {
		err = c.Conn.SetDeadline(time.Now().Add(c.idleTimeout))
		if err != nil {
			return 0, err
		}
	}
	return n, err
}

func (c *connWithIdle) Write(p []byte) (n int, err error) {
	n, err = c.Conn.Write(p)
	// According to net.Conn document:
	// An idle timeout can be implemented by repeatedly extending
	// the deadline after successful Read or Write calls.
	if c.idleTimeout > 0 && n > 0 && err == nil {
		err = c.Conn.SetDeadline(time.Now().Add(c.idleTimeout))
		if err != nil {
			return 0, err
		}
	}
	return n, err
}

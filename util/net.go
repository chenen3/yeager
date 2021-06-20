package util

import (
	"io"
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

// network connection with idle timeout
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

// A connWithReader subverts the net.Conn.Read implementation, primarily so that
// extra bytes can be transparently prepended.
type connWithReader struct {
	net.Conn
	r io.Reader
}

// ConnWithReader by using an io.MultiReader one can define the behaviour of reading from conn and extra data
func ConnWithReader(conn net.Conn, reader io.Reader) *connWithReader {
	return &connWithReader{conn, reader}
}

func (c *connWithReader) Read(b []byte) (n int, err error) {
	return c.r.Read(b)
}

type connPreWrite struct {
	net.Conn
	r io.Reader
}

// ConnWithPreWrite subverts the net.Conn.Write implementation, primarily so that
// extra bytes can be transparently pre-write.
func ConnWithPreWrite(conn net.Conn, reader io.Reader) *connPreWrite {
	return &connPreWrite{conn, reader}
}

func (c *connPreWrite) Write(b []byte) (n int, err error) {
	if c.r != nil {
		_, err = io.Copy(c.Conn, c.r)
		if err != nil {
			return
		}
		c.r = nil
	}
	return c.Conn.Write(b)
}

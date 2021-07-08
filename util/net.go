package util

import (
	"io"
	"net"
	"time"
)

// ChoosePort choose a local port number automatically
func ChoosePort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	ln.Close()
	return ln.Addr().(*net.TCPAddr).Port, nil
}

type maxIdleConn struct {
	net.Conn
	idleTimeout time.Duration
}

func (c *maxIdleConn) Read(b []byte) (n int, err error) {
	n, err = c.Conn.Read(b)
	if c.idleTimeout > 0 && n > 0 && err == nil {
		err = c.Conn.SetDeadline(time.Now().Add(c.idleTimeout))
		if err != nil {
			return 0, err
		}
	}
	return n, err
}

func (c *maxIdleConn) Write(p []byte) (n int, err error) {
	n, err = c.Conn.Write(p)
	if c.idleTimeout > 0 && n > 0 && err == nil {
		err = c.Conn.SetDeadline(time.Now().Add(c.idleTimeout))
		if err != nil {
			return 0, err
		}
	}
	return n, err
}

// NewMaxIdleConn return an connection that implement idle timeout,
// by repeatedly extending the deadline after successful Read or Write calls.
func NewMaxIdleConn(c net.Conn, t time.Duration) net.Conn {
	return &maxIdleConn{
		Conn:        c,
		idleTimeout: t,
	}
}

type earlyReadConn struct {
	net.Conn
	reader io.Reader
}

func (erc *earlyReadConn) Read(b []byte) (n int, err error) {
	return erc.reader.Read(b)
}

// EarlyReadConn returns a net.Conn that subverts the Read implementation,
// it reads from r early before the embed net.Conn
func EarlyReadConn(conn net.Conn, r io.Reader) net.Conn {
	return &earlyReadConn{
		Conn:   conn,
		reader: io.MultiReader(r, conn),
	}
}

type earlyWriteConn struct {
	net.Conn
	reader io.Reader
}

func (ewc *earlyWriteConn) Write(b []byte) (n int, err error) {
	if ewc.reader != nil {
		_, err = io.Copy(ewc.Conn, ewc.reader)
		if err != nil {
			return
		}
		ewc.reader = nil
	}
	return ewc.Conn.Write(b)
}

// EarlyWriteConn returns a net.Conn that subverts the Write implementation,
// it reads from r and write early before the first time calling the embed net.Conn.Write
func EarlyWriteConn(conn net.Conn, r io.Reader) net.Conn {
	return &earlyWriteConn{conn, r}
}

package net

import (
	"errors"
	"io"
	"log/slog"
	"net"
	"sync"
	"time"
)

const (
	DialTimeout      = 5 * time.Second
	HandshakeTimeout = 5 * time.Second
)

var bufPool = sync.Pool{
	New: func() any {
		// refer to 16KB maxPlaintext in crypto/tls/common.go
		s := make([]byte, 16*1024)
		// A pointer can be put into the return interface value without an allocation.
		return &s
	},
}

// Copy is adapted from io.Copy.
// It ignores the ReadFrom and WriteTo Interface,
// stages through the built-in buffer pool
func Copy(dst io.Writer, src io.Reader) (written int64, err error) {
	b := bufPool.Get().(*[]byte)
	for {
		nr, er := src.Read(*b)
		if nr > 0 {
			nw, ew := dst.Write((*b)[0:nr])
			if nw < 0 || nr < nw {
				nw = 0
				if ew == nil {
					ew = errors.New("invalid write result")
				}
			}
			written += int64(nw)
			if ew != nil {
				err = ew
				break
			}
			if nr != nw {
				err = io.ErrShortWrite
				break
			}
		}
		if er != nil {
			if er != io.EOF {
				err = er
			}
			break
		}
	}
	bufPool.Put(b)
	return written, err
}

// Relay copies data in both directions between a and b,
// blocks until one of them completes.
func Relay(a, b io.ReadWriter) error {
	c := make(chan error, 2)
	go func() {
		_, err := Copy(a, b)
		c <- err
	}()
	go func() {
		_, err := Copy(b, a)
		c <- err
	}()
	return <-c
}

// EchoServer accepts connection and writes back anything it reads from the connection.
type EchoServer struct {
	Listener net.Listener
	running  sync.WaitGroup
}

func (e *EchoServer) Serve() {
	e.running.Add(1)
	defer e.running.Done()
	for {
		conn, err := e.Listener.Accept()
		if err != nil {
			if e != nil && !errors.Is(err, net.ErrClosed) {
				slog.Error(err.Error())
			}
			return
		}
		e.running.Add(1)
		go func() {
			defer e.running.Done()
			io.Copy(conn, conn)
			conn.Close()
		}()
	}
}

func (e *EchoServer) Close() error {
	err := e.Listener.Close()
	e.running.Wait()
	return err
}

// StartEchoServer starts an echo server for testing.
func StartEchoServer() (*EchoServer, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	e := EchoServer{Listener: listener}
	go e.Serve()
	return &e, nil
}

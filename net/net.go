package net

import (
	"errors"
	"io"
	"log"
	"net"
	"sync"
	"time"
)

const (
	DialTimeout      = 10 * time.Second
	HandshakeTimeout = 5 * time.Second
	KeepAlive        = 30 * time.Second
	IdleConnTimeout  = 90 * time.Second
)

var bufPool = sync.Pool{
	New: func() any {
		s := make([]byte, 32*1024)
		// A pointer can be put into the return interface value without an allocation.
		return &s
	},
}

// copyBufferPool adapted from copyBuffer in go/src/io/io.go, copies from src to dst.
// The buffer will be fetched from cache or a new one allocated to perform the copy.
func copyBufferPool(dst io.Writer, src io.Reader) (written int64, err error) {
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

// the existence of this function helps to see the explicit name
// of the goroutine when profiling
func oneWayRelay(dst io.Writer, src io.Reader, ch chan<- error) {
	_, err := copyBufferPool(dst, src)
	ch <- err
}

// Relay copies data in both directions between local and remote,
// blocks until one of them completes.
// After Relay returns, the caller should close local and remote,
// so that the goroutine of oneWayRelay will exit as soon as possible.
func Relay(local, remote io.ReadWriter) error {
	// given unbuffered channel will block the left goroutine of oneWayRelay forever
	ch := make(chan error, 2)
	go oneWayRelay(remote, local, ch)
	go oneWayRelay(local, remote, ch)
	return <-ch
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
				log.Printf("failed to accept conn: %v", err)
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

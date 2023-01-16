package net

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/lucas-clemente/quic-go"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	DialTimeout      = 10 * time.Second
	HandshakeTimeout = 5 * time.Second
	KeepAlive        = 30 * time.Second
	IdleConnTimeout  = 90 * time.Second
)

var bufPool = sync.Pool{
	New: func() any {
		// refer to 16KB maxPlaintext in crypto/tls/common.go
		s := make([]byte, 16*1024)
		// A pointer can be put into the return interface value without an allocation.
		return &s
	},
}

type writerOnly struct {
	io.Writer
}

type result struct {
	N   int64
	Err error
}

// oneWayRelay exists just so that profiling show the name of the goroutine
func oneWayRelay(dst io.WriteCloser, src io.Reader, ch chan<- result) {
	var w io.Writer
	if _, ok := dst.(*net.TCPConn); ok {
		// the profiling of goroutine shows that send file is not applicable in this scenario,
		// use wrapper to hide existing TCPConn.ReadFrom from io.CopyBuffer,
		// so that buffer would be reused.
		w = writerOnly{dst}
	} else {
		w = dst
	}
	b := bufPool.Get().(*[]byte)
	n, err := io.CopyBuffer(w, src, *b)
	ch <- result{n, err}
	// unblock Read on dst
	dst.Close()
	bufPool.Put(b)
}

// Relay copies data in both directions between local and remote,
// blocks until one of them completes, returns the number of bytes
// sent to remote and received from remote.
func Relay(local, remote io.ReadWriteCloser) (sent int64, received int64, err error) {
	// must be buffered channel, otherwise the send will not stop immediately
	// after the receive is complete
	sendCh := make(chan result, 1)
	recvCh := make(chan result, 1)
	go oneWayRelay(remote, local, sendCh)
	go oneWayRelay(local, remote, recvCh)
	send := <-sendCh
	recv := <-recvCh
	if send.Err != nil && !closedOrCanceled(send.Err) {
		err = fmt.Errorf("send: %s", send.Err)
	}
	if err != nil && recv.Err != nil && !closedOrCanceled(recv.Err) {
		err = fmt.Errorf("recv: %s", recv.Err)
	}
	return send.N, recv.N, err
}

const ErrCodeCancelRead = 0

// check for closed or canceled error cause by dst.Close() in oneWayRelay
func closedOrCanceled(err error) bool {
	if errors.Is(err, net.ErrClosed) {
		return true
	}
	if errors.Is(err, new(quic.StreamError)) {
		i, _ := err.(*quic.StreamError)
		return i.ErrorCode == ErrCodeCancelRead
	}
	s, ok := status.FromError(err)
	return ok && s != nil && s.Code() == codes.Canceled
}

// ReadableBytes converts the number of bytes into a more readable format.
// For example, given n=1024, returns "1.0KB"
func ReadableBytes(n int64) string {
	switch {
	case n < 1024:
		return strconv.FormatInt(n, 10) + "B"
	case 1024 <= n && n < 1024*1024:
		return strconv.FormatFloat(float64(n)/1024, 'f', 1, 64) + "KB"
	default:
		return strconv.FormatFloat(float64(n)/(1024*1024), 'f', 1, 64) + "MB"
	}
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

package echo

import (
	"errors"
	"io"
	"log/slog"
	"net"
	"sync"
)

// Server accepts connection and writes back anything it reads.
type Server struct {
	Listener net.Listener
	running  sync.WaitGroup
}

func (e *Server) Serve() {
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

func (e *Server) Close() error {
	err := e.Listener.Close()
	e.running.Wait()
	return err
}

// NewServer starts a TCP server for testing,
// it sends back whatever it receives.
func NewServer() *Server {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	e := Server{Listener: listener}
	go e.Serve()
	return &e
}

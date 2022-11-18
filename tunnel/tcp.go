package tunnel

import (
	"context"
	"errors"
	"io"
	"log"
	"net"
	"sync"
	"time"

	"github.com/chenen3/yeager/util"
)

type TcpTunnelServer struct {
	mu          sync.Mutex
	listener    net.Listener
	activeConns map[net.Conn]struct{}
}

// Serve will return a non-nil error unless Close is called.
// TODO: would it be better if given net.Listener instead of address ?
func (srv *TcpTunnelServer) Serve(address string) error {
	lis, err := net.Listen("tcp", address)
	if err != nil {
		return err
	}
	srv.mu.Lock()
	srv.listener = lis
	srv.mu.Unlock()

	for {
		conn, err := lis.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				err = nil
			}
			return err
		}

		srv.trackConn(conn, true)
		go func() {
			defer srv.trackConn(conn, false)
			defer conn.Close()
			conn.SetReadDeadline(time.Now().Add(util.HandshakeTimeout))
			dstAddr, err := ReadHeader(conn)
			if err != nil {
				log.Printf("parse header from peer: %s, error: %s", conn.RemoteAddr(), err)
				// drain the bad connection
				io.Copy(io.Discard, conn)
				return
			}
			conn.SetReadDeadline(time.Time{})

			dstConn, err := net.DialTimeout("tcp", dstAddr, util.DialTimeout)
			if err != nil {
				log.Print(err)
				return
			}
			defer dstConn.Close()
			go io.Copy(dstConn, conn)
			io.Copy(conn, dstConn)
		}()
	}
}

func (srv *TcpTunnelServer) trackConn(c net.Conn, add bool) {
	srv.mu.Lock()
	defer srv.mu.Unlock()
	if srv.activeConns == nil {
		srv.activeConns = make(map[net.Conn]struct{})
	}
	if add {
		srv.activeConns[c] = struct{}{}
	} else {
		delete(srv.activeConns, c)
	}
}

// Close closes the TCP tunnel server. It immediately
// closes all active connections and listener
func (srv *TcpTunnelServer) Close() error {
	srv.mu.Lock()
	defer srv.mu.Unlock()
	var err error
	if srv.listener != nil {
		err = srv.listener.Close()
	}

	for c := range srv.activeConns {
		c.Close()
		delete(srv.activeConns, c)
	}
	return err
}

type TcpTunnelClient struct {
	address string
}

func NewTcpTunnelClient(address string) *TcpTunnelClient {
	return &TcpTunnelClient{address: address}
}

func (tc *TcpTunnelClient) DialContext(ctx context.Context, dstAddr string) (io.ReadWriteCloser, error) {
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", tc.address)
	if err != nil {
		return nil, err
	}

	header, err := MakeHeader(dstAddr)
	if err != nil {
		return nil, err
	}
	_, err = conn.Write(header)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

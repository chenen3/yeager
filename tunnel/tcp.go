package tunnel

import (
	"context"
	"errors"
	"io"
	"log"
	"net"
	"time"

	"github.com/chenen3/yeager/util"
)

type TcpTunnel struct {
	Address  string
	listener net.Listener
}

// Serve will return a non-nil error unless Close is called.
func (t *TcpTunnel) Serve() error {
	lis, err := net.Listen("tcp", t.Address)
	if err != nil {
		return err
	}
	t.listener = lis

	for {
		conn, err := lis.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				err = nil
			}
			return err
		}
		go func() {
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

func (t *TcpTunnel) DialContext(ctx context.Context, dstAddr string) (io.ReadWriteCloser, error) {
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", t.Address)
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

func (t *TcpTunnel) Close() error {
	if t.listener != nil {
		return t.listener.Close()
	}
	return nil
}

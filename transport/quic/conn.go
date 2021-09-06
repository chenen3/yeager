package quic

import (
	"net"

	"github.com/lucas-clemente/quic-go"
)

// conn implement net.Conn interface
type conn struct {
	quic.Stream
	localAddr  net.Addr
	remoteAddr net.Addr
}

func (c *conn) LocalAddr() net.Addr {
	return c.localAddr
}

func (c *conn) RemoteAddr() net.Addr {
	return c.remoteAddr
}

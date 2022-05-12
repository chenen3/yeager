package quic

import (
	"net"

	"github.com/lucas-clemente/quic-go"
)

// streamConn wraps quic.Stream and implements net.Conn interface
type streamConn struct {
	quic.Stream
	localAddr  net.Addr
	remoteAddr net.Addr
}

func (sc *streamConn) LocalAddr() net.Addr {
	return sc.localAddr
}

func (sc *streamConn) RemoteAddr() net.Addr {
	return sc.remoteAddr
}

func (sc *streamConn) Close() error {
	// here need to close both read-direction and write-direction of the stream,
	// while quic.Stream.Close() only closes the write-direction
	sc.CancelRead(0)
	return sc.Stream.Close()
}

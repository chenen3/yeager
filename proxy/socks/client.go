package socks

import (
	"errors"
	"fmt"
	"io"
	"net"

	"github.com/chenen3/yeager/proxy/common"
	"github.com/chenen3/yeager/util"
)

// socks5 dialer, test only
type dialer struct {
	ServerAddr string
}

func (d *dialer) Dial(network, addr string) (net.Conn, error) {
	var cmd int
	switch network {
	case "tcp":
		cmd = cmdConnect
	case "udp":
		cmd = cmdUDPAssociate
	default:
		return nil, errors.New("unsupported network: " + network)
	}

	tc, err := net.Dial("tcp", d.ServerAddr)
	if err != nil {
		return nil, err
	}

	// write VER NMETHODS METHODS
	if _, err = tc.Write([]byte{0x05, 0x01, 0x00}); err != nil {
		return nil, err
	}

	// read VER METHOD
	var b [2]byte
	if _, err = io.ReadFull(tc, b[:]); err != nil {
		return nil, err
	}
	method := b[1]
	if method != noAuth {
		return nil, fmt.Errorf("unsupported method %x", method)
	}

	// write VER REP RSV ATYP BND.ADDR BND.PORT
	dst, err := util.ParseAddr("tcp", addr)
	if err != nil {
		return nil, err
	}
	cmdReq := append([]byte{0x05, byte(cmd), 0x00}, common.MarshalAddr(dst)...)
	if _, err = tc.Write(cmdReq); err != nil {
		return nil, err
	}

	// read RSV FRAG ATYP DST.ADDR DST.PORT DATA
	var bs [3]byte
	if _, err = io.ReadFull(tc, bs[:]); err != nil {
		return nil, err
	}

	if reply := bs[1]; reply != 0x00 {
		return nil, fmt.Errorf("receive failed reply code: %x", reply)
	}
	boundAddr, err := common.ReadAddr(tc)
	if err != nil {
		return nil, err
	}

	if cmd == cmdUDPAssociate {
		uc, err := net.Dial("udp", boundAddr)
		if err != nil {
			return nil, err
		}
		return &ClientUDPConn{
			UDPConn: uc.(*net.UDPConn),
			Dst:     dst,
			onClose: func() {
				tc.Close()
			},
		}, nil
	}

	return tc, nil
}

type ClientUDPConn struct {
	*net.UDPConn
	Dst     *util.Addr
	data    []byte
	off     int
	onClose func()
}

func (c *ClientUDPConn) Read(b []byte) (int, error) {
	if c.off >= len(c.data) {
		buf := pool.Get().([]byte)
		defer pool.Put(buf)
		n, err := c.UDPConn.Read(buf)
		if err != nil {
			return 0, err
		}

		var dg datagram
		if err := dg.Unmarshal(buf[:n]); err != nil {
			return 0, err
		}

		c.data = dg.data
		c.off = 0
	}

	n := copy(b, c.data[c.off:])
	c.off += n
	return n, nil
}

func (c *ClientUDPConn) Write(b []byte) (int, error) {
	dg := &datagram{
		dst:  c.Dst,
		data: b,
	}
	bs, err := dg.Marshal()
	if err != nil {
		return 0, err
	}
	return c.UDPConn.Write(bs)
}

func (c *ClientUDPConn) Close() error {
	if c.onClose != nil {
		c.onClose()
	}
	return c.UDPConn.Close()
}

package socks

import (
	"errors"
	"fmt"
	"io"
	"net"

	"github.com/chenen3/yeager/util"
)

// socks5 dialer, test only
type dialer struct {
	ServerAddr string
}

func (d *dialer) Dial(network, addr string) (net.Conn, error) {
	var cmd command
	switch network {
	case "tcp":
		cmd = cmdConnect
	case "udp":
		cmd = cmdUDP
	default:
		return nil, errors.New("unsupported network: " + network)
	}

	c, err := net.Dial("tcp", d.ServerAddr)
	if err != nil {
		return nil, err
	}

	// send auth request and receive reply
	authReq := AuthRequest{version: ver5, nMethods: 0x01, methods: []byte{0x00}}
	_, err = c.Write(authReq.Marshal())
	if err != nil {
		return nil, err
	}

	var b [2]byte
	_, err = io.ReadFull(c, b[:])
	if err != nil {
		return nil, err
	}
	var authReply AuthReply
	err = authReply.Unmarshal(b[:])
	if err != nil {
		return nil, err
	}
	if authReply.method != noAuth {
		return nil, fmt.Errorf("unsupported auth method: %x", authReply.method)
	}

	// send cmd request and receive reply
	a, err := util.ParseAddress(addr)
	if err != nil {
		return nil, err
	}
	cmdReq := CmdRequest{version: ver5, cmd: cmd, Address: a}
	bs, err := cmdReq.Marshal()
	if err != nil {
		return nil, err
	}
	_, err = c.Write(bs)
	if err != nil {
		return nil, err
	}

	cmdReply, err := parseCmdReply(c)
	if err != nil {
		return nil, err
	}
	if cmdReply.code != success {
		return nil, fmt.Errorf("receive failed reply code: %x", cmdReply.code)
	}

	if cmd == cmdUDP {
		c.Close() // TODO: close this tcp connection when udp connection end
		uc, err := net.Dial("udp", d.ServerAddr)
		if err != nil {
			return nil, err
		}
		return &ClientUDPConn{UDPConn: uc.(*net.UDPConn), Dst: a}, nil
	}
	return c, nil
}

type ClientUDPConn struct {
	*net.UDPConn
	Dst  *util.Address
	data []byte
	off  int
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

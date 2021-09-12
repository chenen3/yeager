package socks

import (
	"errors"
	"fmt"
	"io"
	"net"
	"yeager/proxy"
)

// socks5 proxy dialer, test only
type dialer struct {
	proxyAddress string
}

func (d *dialer) dial(network, addr string) (net.Conn, error) {
	var cmd command
	switch network {
	case "tcp":
		cmd = cmdConnect
	case "udp":
		cmd = cmdUDP
	default:
		return nil, errors.New("unsupported network: " + network)
	}

	c, err := net.Dial("tcp", d.proxyAddress)
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
	a, err := proxy.ParseAddress(network, addr)
	if err != nil {
		return nil, err
	}
	cmdReq := CmdRequest{version: ver5, cmd: cmd, addr: a}
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
	if cmdReply.reply != success {
		return nil, fmt.Errorf("receive failed reply: %x", cmdReply.reply)
	}

	if cmd == cmdUDP {
		uc, err := net.Dial("udp", cmdReply.addr.String())
		if err != nil {
			return nil, err
		}
		c = &ClientUDPConn{UDPConn: uc.(*net.UDPConn), dst: a}
	}
	return c, nil
}

type ClientUDPConn struct {
	*net.UDPConn
	dst *proxy.Address
}

func (c *ClientUDPConn) Read(b []byte) (int, error) {
	// TODO: buffer pool
	buf := make([]byte, 1024)
	n, err := c.UDPConn.Read(buf)
	if err != nil {
		return 0, err
	}

	dg, err := parseDatagram(buf[:n])
	if err != nil {
		return 0, err
	}

	n = copy(b, dg.data)
	if n < len(dg.data) {
		return n, errors.New("short read")
	}
	return n, nil
}

func (c *ClientUDPConn) Write(b []byte) (int, error) {
	return c.UDPConn.Write(marshalDatagram(c.dst, b))
}

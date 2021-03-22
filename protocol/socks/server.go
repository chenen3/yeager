package socks

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"strconv"

	"yeager/log"
	"yeager/protocol"
)

// Conn is an implementation of the protocol.Conn interface for network connections.
type Conn struct {
	net.Conn
	dstAddr net.Addr
}

func NewConn(conn net.Conn, dstAddr net.Addr) *Conn {
	return &Conn{Conn: conn, dstAddr: dstAddr}
}

func (c *Conn) DstAddr() net.Addr {
	return c.dstAddr
}

type Server struct {
	conf   *Config
	connCh chan *Conn
	ctx    context.Context
	cancel context.CancelFunc
}

func NewServer(config *Config) (*Server, error) {
	s := &Server{
		conf:   config,
		connCh: make(chan *Conn, 32),
	}
	s.ctx, s.cancel = context.WithCancel(context.Background())

	go func() {
		log.Error(s.listenAndServe())
		s.cancel()
	}()
	return s, nil
}

func (s *Server) Accept() (protocol.Conn, error) {
	select {
	case <-s.ctx.Done():
		// 以防信道里面还有连接未取走
		select {
		case conn := <-s.connCh:
			return conn, nil
		default:
			return nil, errors.New("server closed")
		}
	case conn := <-s.connCh:
		return conn, nil
	}
}

func (s *Server) Close() error {
	s.cancel()
	return nil
}

func (s *Server) listenAndServe() error {
	addr := net.JoinHostPort(s.conf.Host, strconv.Itoa(s.conf.Port))
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	defer ln.Close()

	for {
		select {
		case <-s.ctx.Done():
			return s.ctx.Err()
		default:
		}

		conn, err := ln.Accept()
		if err != nil {
			log.Error(err)
			continue
		}
		go s.handleConnection(conn)
	}
}

func (s *Server) handleConnection(conn net.Conn) {
	dstAddr, err := s.handshake(conn)
	if err != nil {
		log.Error(err)
		conn.Close()
		return
	}
	s.connCh <- NewConn(conn, dstAddr)
}

func (s *Server) handshake(conn net.Conn) (dst net.Addr, err error) {
	err = s.socksAuth(conn)
	if err != nil {
		return nil, err
	}
	return s.socksConnect(conn)
}

const (
	ver5       = 0x05 // socks5 版本号
	noAuth     = 0x00
	cmdConnect = 0x01
	atypIPv4   = 0x01
	atypDomain = 0x03
	atypIPv6   = 0x04
)

func (s *Server) socksAuth(conn net.Conn) error {
	/*
		客户端第一次请求格式(以字节为单位):
		VER	NMETHODS	METHODS
		1	1			1-255
	*/
	var buf [2]byte
	_, err := io.ReadFull(conn, buf[:])
	if err != nil {
		return errors.New("reading header: %s" + err.Error())
	}
	ver, nMethods := buf[0], buf[1]
	if ver != ver5 {
		return fmt.Errorf("unsupported VER: %d", buf[0])
	}
	_, err = io.CopyN(ioutil.Discard, conn, int64(nMethods))
	if err != nil {
		return errors.New("reading METHODS: " + err.Error())
	}
	/*
		服务端第一次回复格式(以字节为单位):
		VER	METHOD
		1	1
	*/
	// socks5服务在此仅作为入站代理，使用场景应该是本地内网，无需认证
	_, err = conn.Write([]byte{ver5, noAuth})
	if err != nil {
		return errors.New("conn write: " + err.Error())
	}
	return nil
}

func (s *Server) socksConnect(conn net.Conn) (dstAddr net.Addr, err error) {
	var buf [4]byte
	/*
		客户端第二次请求格式(以字节为单位):
		VER	CMD	RSV		ATYP	DST.ADDR	DST.PORT
		1	1	0x00	1		动态			2
	*/
	_, err = io.ReadFull(conn, buf[:])
	if err != nil {
		return nil, errors.New("reading request: %s" + err.Error())
	}

	ver, cmd, atyp := buf[0], buf[1], buf[3]
	if ver != ver5 {
		return nil, fmt.Errorf("unsupported VER: %d", buf[0])
	}
	if cmd != cmdConnect {
		// 若后续有需要，再支持余下的BIND与UDP
		return nil, fmt.Errorf("unsupported CMD: %d", buf[1])
	}

	var addr string
	switch atyp {
	case atypIPv4:
		var buf [4]byte
		_, err = io.ReadFull(conn, buf[:])
		if err != nil {
			return nil, err
		}
		addr = net.IPv4(buf[0], buf[1], buf[2], buf[3]).String()
	case atypDomain:
		var buf [1]byte
		_, err = io.ReadFull(conn, buf[:])
		if err != nil {
			return nil, err
		}
		length := buf[0]

		bs := make([]byte, length)
		_, err := io.ReadFull(conn, bs)
		if err != nil {
			return nil, err
		}
		addr = string(bs)
	case atypIPv6:
		return nil, errors.New("IPv6 not supported yet")
	default:
		return nil, fmt.Errorf("unknown atyp: %x", buf[3])
	}

	var portBuf [2]byte
	_, err = io.ReadFull(conn, portBuf[:])
	if err != nil {
		return nil, err
	}
	port := binary.BigEndian.Uint16(portBuf[:])

	dstAddr, err = net.ResolveTCPAddr("tcp", fmt.Sprintf("%s:%d", addr, port))
	if err != nil {
		return nil, err
	}
	/*
		服务器第二次回复格式（以字节为单位）：
		VER	REP	RSV		ATYP	BND.ADDR	BND.PORT
		1	1	0x00	1		动态			2
	*/
	_, err = conn.Write([]byte{0x05, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})
	if err != nil {
		return nil, err
	}

	return dstAddr, nil
}

package socks

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	glog "log"
	"net"
	"strconv"
	"time"

	"yeager/log"
	"yeager/proxy"
)

// Server implements protocol.Inbound interface
type Server struct {
	conf        *Config
	ready       chan struct{}
	handlerFunc func(context.Context, net.Conn, *proxy.Address)
}

func NewServer(config *Config) *Server {
	return &Server{
		conf:  config,
		ready: make(chan struct{}),
	}
}

func (s *Server) ListenAndServe(ctx context.Context) error {
	addr := net.JoinHostPort(s.conf.Host, strconv.Itoa(s.conf.Port))
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("socks5 proxy failed to listen, err: %s", err)
	}
	defer ln.Close()
	glog.Println("socks5 proxy listening on", addr)

	close(s.ready)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		conn, err := ln.Accept()
		if err != nil {
			log.Error(err)
			continue
		}
		go s.handleConnection(ctx, conn)
	}
}

func (s *Server) RegisterHandler(handlerFunc func(context.Context, net.Conn, *proxy.Address)) {
	s.handlerFunc = handlerFunc
}

func (s *Server) handleConnection(ctx context.Context, conn net.Conn) {
	dstAddr, err := s.handshake(conn)
	if err != nil {
		log.Error("failed to handshake: " + err.Error())
		conn.Close()
		return
	}

	s.handlerFunc(ctx, conn, dstAddr)
}

func (s *Server) handshake(conn net.Conn) (dst *proxy.Address, err error) {
	err = conn.SetDeadline(time.Now().Add(proxy.HandshakeTimeout))
	if err != nil {
		return
	}
	defer func() {
		er := conn.SetDeadline(time.Time{})
		if er != nil && err == nil {
			err = er
		}
	}()

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
		return errors.New("reading header: " + err.Error())
	}
	ver, nMethods := buf[0], buf[1]
	if ver != ver5 {
		return fmt.Errorf("unsupported VER: %d", buf[0])
	}
	_, err = io.CopyN(io.Discard, conn, int64(nMethods))
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

func (s *Server) socksConnect(conn net.Conn) (dstAddr *proxy.Address, err error) {
	var buf [4]byte
	/*
		客户端第二次请求格式(以字节为单位):
		VER	CMD	RSV		ATYP	DST.ADDR	DST.PORT
		1	1	0x00	1		动态			2
	*/
	_, err = io.ReadFull(conn, buf[:])
	if err != nil {
		return nil, errors.New("reading request: " + err.Error())
	}

	ver, cmd, atyp := buf[0], buf[1], buf[3]
	if ver != ver5 {
		return nil, fmt.Errorf("unsupported VER: %d", buf[0])
	}
	if cmd != cmdConnect {
		// 若后续有需要，再支持余下的BIND与UDP
		return nil, fmt.Errorf("unsupported CMD: %d", buf[1])
	}

	var host string
	switch atyp {
	case atypIPv4:
		var buf [4]byte
		_, err = io.ReadFull(conn, buf[:])
		if err != nil {
			return nil, err
		}
		host = net.IPv4(buf[0], buf[1], buf[2], buf[3]).String()
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
		host = string(bs)
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

	/*
		服务器第二次回复格式（以字节为单位）：
		VER	REP	RSV		ATYP	BND.ADDR	BND.PORT
		1	1	0x00	1		动态			2
	*/
	_, err = conn.Write([]byte{0x05, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})
	if err != nil {
		return nil, err
	}

	dstAddr = proxy.NewAddress(host, int(port))
	return dstAddr, nil
}

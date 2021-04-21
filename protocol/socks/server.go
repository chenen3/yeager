package socks

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	glog "log"
	"net"
	"strconv"

	"github.com/opentracing/opentracing-go"
	"yeager/log"
	"yeager/protocol"
)

type Server struct {
	conf   *Config
	connCh chan protocol.Conn
	ctx    context.Context
	cancel context.CancelFunc
}

func NewServer(config *Config) (*Server, error) {
	s := &Server{
		conf:   config,
		connCh: make(chan protocol.Conn, 32),
	}
	s.ctx, s.cancel = context.WithCancel(context.Background())

	go func() {
		log.Error(s.listenAndServe())
		s.cancel()
	}()
	return s, nil
}

func (s *Server) Accept() <-chan protocol.Conn {
	return s.connCh
}

func (s *Server) Close() error {
	s.cancel()
	close(s.connCh)
	return nil
}

func (s *Server) listenAndServe() error {
	ln, err := net.Listen("tcp", net.JoinHostPort(s.conf.Host, strconv.Itoa(s.conf.Port)))
	if err != nil {
		return err
	}
	defer ln.Close()
	glog.Println("SOCKS5 proxy server listening", net.JoinHostPort(s.conf.Host, strconv.Itoa(s.conf.Port)))

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
	span := opentracing.StartSpan("sock-inbound")
	defer span.Finish()

	dstAddr, err := s.handshake(conn)
	if err != nil {
		log.Error(err)
		conn.Close()
		return
	}
	span.SetTag("addr", dstAddr.String())

	newConn := protocol.NewConn(conn, dstAddr)
	// in case send on closed channel
	select {
	case <-s.ctx.Done():
		conn.Close()
		return
	case s.connCh <- newConn:
	}
}

func (s *Server) handshake(conn net.Conn) (dst *protocol.Address, err error) {
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

func (s *Server) socksConnect(conn net.Conn) (dstAddr *protocol.Address, err error) {
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

	dstAddr = protocol.NewAddress(host, int(port))
	return dstAddr, nil
}

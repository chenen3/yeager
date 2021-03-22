package yeager

import (
	"context"
	"crypto/tls"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strconv"

	"github.com/google/uuid"
	"yeager/log"
	"yeager/protocol"
)

type Server struct {
	conf   *ServerConfig
	connCh chan protocol.Conn
	ctx    context.Context
	cancel context.CancelFunc
}

func NewServer(config *ServerConfig) (*Server, error) {
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
	cert, err := tls.X509KeyPair(s.conf.certPEMBlock, s.conf.keyPEMBlock)
	if err != nil {
		return err
	}
	config := &tls.Config{
		Certificates: []tls.Certificate{cert},
	}
	addr := net.JoinHostPort(s.conf.Host, strconv.Itoa(s.conf.Port))
	ln, err := tls.Listen("tcp", addr, config)
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

	// in case send on closed channel
	select {
	case <-s.ctx.Done():
		conn.Close()
		return
	case s.connCh <- protocol.NewConn(conn, dstAddr):
	}
}

func (s *Server) handshake(conn net.Conn) (dstAddr net.Addr, err error) {
	/*
		客户端请求格式，仿照socks5协议(以字节为单位):
		UUID	ATYP	DST.ADDR	DST.PORT
		36		1		动态			2
	*/
	var buf [37]byte
	_, err = io.ReadFull(conn, buf[:])
	if err != nil {
		return nil, err
	}
	uuidBytes, atyp := buf[:36], buf[36]
	gotUUID, err := uuid.ParseBytes(uuidBytes)
	if err != nil {
		return nil, err
	}
	wantUUID, err := uuid.Parse(s.conf.UUID)
	if err != nil {
		return nil, err
	}
	// TODO fallback
	if gotUUID != wantUUID {
		return nil, fmt.Errorf("want uuid: %s, got: %s", wantUUID, gotUUID)
	}

	var addr string
	switch atyp {
	case addressIPv4:
		var buf [4]byte
		_, err = io.ReadFull(conn, buf[:])
		if err != nil {
			return nil, err
		}
		addr = net.IPv4(buf[0], buf[1], buf[2], buf[3]).String()
	case addressDomain:
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
	default:
		return nil, fmt.Errorf("unsupported address type: %x", atyp)
	}

	var bs [2]byte
	_, err = io.ReadFull(conn, bs[:])
	if err != nil {
		return nil, err
	}
	port := binary.BigEndian.Uint16(bs[:])

	dstAddr, err = net.ResolveTCPAddr("tcp", fmt.Sprintf("%s:%d", addr, port))
	if err != nil {
		return nil, err
	}
	/*
		服务端回应格式(以字节为单位):
		VER	REP
		1	1
	*/
	_, err = conn.Write([]byte{versionBeta, responseSuccess})
	if err != nil {
		return nil, err
	}

	return dstAddr, nil
}

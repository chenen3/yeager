// Package socks provides a SOCKS version 5 server implementation.
package socks

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"yeager/config"
	"yeager/log"
	"yeager/proxy"
)

// Server implements protocol.Inbound interface
type Server struct {
	ctx    context.Context
	cancel context.CancelFunc
	conf   *config.SOCKSProxy
	lis    net.Listener
	wg     sync.WaitGroup // counts active Serve goroutines for graceful close

	ready chan struct{} // imply that server is ready to accept connection, testing only
}

func NewServer(config *config.SOCKSProxy) *Server {
	ctx, cancel := context.WithCancel(context.Background())
	return &Server{
		conf:   config,
		ready:  make(chan struct{}),
		ctx:    ctx,
		cancel: cancel,
	}
}

func (s *Server) ListenAndServe(handle proxy.Handler) error {
	lis, err := net.Listen("tcp", s.conf.Address)
	if err != nil {
		return fmt.Errorf("socks5 proxy failed to listen, err: %s", err)
	}
	s.lis = lis
	log.Infof("socks5 tcp proxy listening on %s", s.conf.Address)

	close(s.ready)
	for {
		conn, err := lis.Accept()
		if err != nil {
			select {
			case <-s.ctx.Done():
				return nil
			default:
			}
			log.Warn(err)
			continue
		}

		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			addr, err := s.handshake(conn)
			if err != nil {
				log.Error("handshake: " + err.Error())
				conn.Close()
				return
			}

			if addr.Network() == "udp" {
				s.handleUDP(conn)
				return
			}

			handle(s.ctx, conn, addr)
		}()
	}
}

func (s *Server) handleUDP(conn net.Conn) {
	defer conn.Close()
	done := make(chan struct{})
	go func() {
		// according to SOCKS5 protocol, keep this TCP connection until closed
		for {
			var b [1]byte
			_, err := conn.Read(b[:])
			if err, ok := err.(net.Error); ok && err.Timeout() {
				continue
			}
			close(done)
			return
		}
	}()

	select {
	case <-s.ctx.Done():
	case <-done:
	}
}

func (s *Server) Close() error {
	defer s.wg.Wait()
	s.cancel()
	return s.lis.Close()
}

func (s *Server) handshake(conn net.Conn) (addr *proxy.Address, err error) {
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
	_, err = conn.Write([]byte{ver5, byte(noAuth)})
	if err != nil {
		return errors.New("conn write: " + err.Error())
	}
	return nil
}

func (s *Server) socksConnect(conn net.Conn) (addr *proxy.Address, err error) {
	var buf [4]byte
	/*
		客户端第二次请求格式(以字节为单位):
		VER	CMD	RSV		ATYP	DST.ADDR	DST.PORT
		1	1	0x00	1		动态			2
	*/
	_, err = io.ReadFull(conn, buf[:])
	if err != nil {
		err = errors.New("reading request: " + err.Error())
		return
	}

	ver, cmd, atyp := buf[0], buf[1], buf[3]
	if ver != ver5 {
		err = fmt.Errorf("unsupported VER: %d", ver)
		return
	}
	var isUDP bool
	switch command(cmd) {
	case cmdConnect:
	case cmdUDP:
		isUDP = true
	default:
		err = fmt.Errorf("unsupported CMD: %d", cmd)
		return
	}

	var host string
	switch addressType(atyp) {
	case atypIPv4:
		var buf [4]byte
		_, err = io.ReadFull(conn, buf[:])
		if err != nil {
			return
		}
		host = net.IPv4(buf[0], buf[1], buf[2], buf[3]).String()
	case atypDomain:
		var buf [1]byte
		_, err = io.ReadFull(conn, buf[:])
		if err != nil {
			return
		}
		length := buf[0]

		bs := make([]byte, length)
		_, err = io.ReadFull(conn, bs)
		if err != nil {
			return
		}
		host = string(bs)
	case atypIPv6:
		err = errors.New("IPv6 not supported yet")
		return
	default:
		err = fmt.Errorf("unknown addressType: %x", buf[3])
		return
	}

	var portBuf [2]byte
	_, err = io.ReadFull(conn, portBuf[:])
	if err != nil {
		return
	}
	port := binary.BigEndian.Uint16(portBuf[:])

	/*
		服务器第二次回复格式（以字节为单位）：
		VER	REP	RSV		ATYP	BND.ADDR	BND.PORT
		1	1	0x00	1		动态			2
	*/
	// -       _, err = conn.Write([]byte{0x05, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})
	reply := []byte{ver5, 0x00, 0x00}
	reply = append(reply, byte(atypIPv4))
	// TODO: 如果配置文件填写 0.0.0.0:1080，这时对于udp来说，如何返回绑定地址?
	bindAddr, err := proxy.ParseAddress("tcp", s.conf.Address)
	if err != nil {
		return
	}
	reply = append(reply, bindAddr.IP...)
	var bindPort [2]byte
	binary.BigEndian.PutUint16(bindPort[:], uint16(bindAddr.Port))
	reply = append(reply, bindPort[:]...)

	_, err = conn.Write(reply)
	if err != nil {
		return
	}

	network := "tcp"
	if isUDP {
		network = "udp"
	}
	addr, err = proxy.ParseHostPort(network, host, int(port))
	return addr, err
}

// UDPServer implements protocol.Inbound interface
type UDPServer struct {
	ctx    context.Context
	cancel context.CancelFunc
	conf   *config.SOCKSProxy
	wg     sync.WaitGroup // counts active Serve goroutines for graceful close

	ready chan struct{} // imply that server is ready to accept connection, testing only
}

func NewUDPServer(config *config.SOCKSProxy) *UDPServer {
	ctx, cancel := context.WithCancel(context.Background())
	return &UDPServer{
		conf:   config,
		ready:  make(chan struct{}),
		ctx:    ctx,
		cancel: cancel,
	}
}

func (us *UDPServer) ListenAndServe(handle proxy.Handler) error {
	pc, err := net.ListenPacket("udp", us.conf.Address)
	if err != nil {
		return nil
	}
	defer pc.Close()
	log.Infof("socks5 udp proxy listening on %s", us.conf.Address)

	close(us.ready)
	for {
		// TODO: buffer pool
		buf := make([]byte, 1024)
		n, raddr, err := pc.ReadFrom(buf)
		if err != nil {
			select {
			case <-us.ctx.Done():
				return nil
			default:
				log.Error(err)
				continue
			}
		}

		us.wg.Add(1)
		go func() {
			defer us.wg.Done()
			udpConn, err := newUDPConn(pc, raddr, buf[:n])
			if err != nil {
				log.Error(err)
				return
			}
			handle(us.ctx, udpConn, udpConn.datagram.dst)
		}()
	}
}

func (us *UDPServer) Close() error {
	defer us.wg.Wait()
	us.cancel()
	return nil
}

// ServerUDPConn implement net.Conn interface
type ServerUDPConn struct {
	net.PacketConn
	raddr    net.Addr
	datagram *datagram
	off      int
}

func newUDPConn(pc net.PacketConn, raddr net.Addr, req []byte) (*ServerUDPConn, error) {
	dg, err := parseDatagram(req)
	if err != nil {
		return nil, err
	}

	return &ServerUDPConn{
		PacketConn: pc,
		raddr:      raddr,
		datagram:   dg,
	}, nil
}

func (uc *ServerUDPConn) Read(b []byte) (int, error) {
	if uc.off >= len(uc.datagram.data) {
		return 0, io.EOF
	}

	n := copy(b, uc.datagram.data[uc.off:])
	uc.off += n
	return n, nil
}

func (uc *ServerUDPConn) Write(b []byte) (int, error) {
	return uc.PacketConn.WriteTo(marshalDatagram(uc.datagram.dst, b), uc.raddr)
}

func (uc *ServerUDPConn) RemoteAddr() net.Addr {
	return uc.raddr
}

func (uc *ServerUDPConn) Close() error {
	return nil
}

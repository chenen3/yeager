// Package socks provides a SOCKS version 5 server implementation.
package socks

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/chenen3/yeager/config"
	"github.com/chenen3/yeager/log"
	"github.com/chenen3/yeager/proxy/common"
)

// TCPServer implements protocol.Inbound interface
type TCPServer struct {
	ctx    context.Context
	cancel context.CancelFunc
	conf   *config.SOCKSProxy
	lis    net.Listener
	wg     sync.WaitGroup // counts active Serve goroutines for graceful close

	ready chan struct{} // imply that server is ready to accept connection, testing only
}

func NewTCPServer(conf *config.SOCKSProxy) (*TCPServer, error) {
	if conf == nil || conf.Listen == "" {
		return nil, errors.New("config missing listening address")
	}

	ctx, cancel := context.WithCancel(context.Background())
	return &TCPServer{
		conf:   conf,
		ready:  make(chan struct{}),
		ctx:    ctx,
		cancel: cancel,
	}, nil
}

func (s *TCPServer) Name() string {
	return "tcpSOCKS5Server"
}

func (s *TCPServer) ListenAndServe(handle func(ctx context.Context, conn net.Conn, network, addr string)) error {
	lis, err := net.Listen("tcp", s.conf.Listen)
	if err != nil {
		return fmt.Errorf("socks5 proxy failed to listen, err: %s", err)
	}
	s.lis = lis
	log.L().Infof("socks5 TCP proxy listening %s", s.conf.Listen)

	close(s.ready)
	for {
		conn, err := lis.Accept()
		if err != nil {
			select {
			case <-s.ctx.Done():
				return nil
			default:
			}
			log.L().Warnf(err.Error())
			continue
		}

		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			network, addr, err := s.handshake(conn)
			if err != nil {
				log.L().Error("handshake: " + err.Error())
				conn.Close()
				return
			}

			// according to SOCKS5 protocol, while desire UDP,
			// keep this TCP connection until closed by peer
			if network == "udp" {
				s.holdForUDP(conn)
				return
			}

			handle(s.ctx, conn, network, addr)
		}()
	}
}

func (s *TCPServer) holdForUDP(conn net.Conn) {
	defer conn.Close()
	done := make(chan struct{})
	go func() {
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

func (s *TCPServer) Close() error {
	defer s.wg.Wait()
	s.cancel()
	if s.lis != nil {
		return s.lis.Close()
	}
	return nil
}

func (s *TCPServer) handshake(conn net.Conn) (network, addr string, err error) {
	err = conn.SetDeadline(time.Now().Add(common.HandshakeTimeout))
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
		return "", "", err
	}
	return s.socksConnect(conn)
}

func (s *TCPServer) socksAuth(conn net.Conn) error {
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

func (s *TCPServer) socksConnect(conn net.Conn) (network, addr string, err error) {
	var buf [4]byte
	/*
		客户端第二次请求格式(以字节为单位):
		VER	CMD	RSV		ATYP	DST.ADDR	DST.PORT
		1	1	0x00	1		动态			2
	*/
	_, err = io.ReadFull(conn, buf[:])
	if err != nil {
		return "", "", errors.New("reading request: " + err.Error())
	}

	ver, cmd, atyp := buf[0], buf[1], buf[3]
	if ver != ver5 {
		return "", "", fmt.Errorf("unsupported VER: %d", buf[0])
	}

	var isUDP bool
	switch command(cmd) {
	case cmdConnect:
	case cmdUDP:
		isUDP = true
	default:
		return "", "", fmt.Errorf("unsupported CMD: %d", cmd)
	}

	var host string
	switch addressType(atyp) {
	case atypIPv4:
		var buf [4]byte
		_, err = io.ReadFull(conn, buf[:])
		if err != nil {
			return "", "", err
		}
		host = net.IPv4(buf[0], buf[1], buf[2], buf[3]).String()
	case atypDomain:
		var buf [1]byte
		_, err = io.ReadFull(conn, buf[:])
		if err != nil {
			return "", "", err
		}
		length := buf[0]

		bs := make([]byte, length)
		_, err := io.ReadFull(conn, bs)
		if err != nil {
			return "", "", err
		}
		host = string(bs)
	case atypIPv6:
		return "", "", errors.New("IPv6 not supported yet")
	default:
		return "", "", fmt.Errorf("unknown atyp: %x", buf[3])
	}

	var portBuf [2]byte
	_, err = io.ReadFull(conn, portBuf[:])
	if err != nil {
		return "", "", err
	}
	port := binary.BigEndian.Uint16(portBuf[:])

	/*
		服务器第二次回复格式（以字节为单位）：
		VER	REP	RSV		ATYP	BND.ADDR	BND.PORT
		1	1	0x00	1		动态			2
	*/
	// _, err = conn.Write([]byte{0x05, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})

	cmdReply, err := NewCmdReply(s.conf.Listen)
	if err != nil {
		return "", "", err
	}
	// 服务器第二次回复
	_, err = conn.Write(cmdReply.Marshal())
	if err != nil {
		return "", "", err
	}

	network = "tcp"
	if isUDP {
		network = "udp"
	}
	addr = net.JoinHostPort(host, strconv.Itoa(int(port)))
	return network, addr, nil
}

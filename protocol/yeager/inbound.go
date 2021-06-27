package yeager

import (
	"crypto/tls"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	glog "log"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/google/uuid"
	"yeager/log"
	"yeager/protocol"
	"yeager/util"
)

type Server struct {
	conf   *ServerConfig
	connCh chan protocol.Conn
	done   chan struct{}
}

func NewServer(config *ServerConfig) *Server {
	return &Server{
		conf:   config,
		connCh: make(chan protocol.Conn, 32),
		done:   make(chan struct{}),
	}
}

func (s *Server) Accept() <-chan protocol.Conn {
	return s.connCh
}

func (s *Server) Close() error {
	close(s.done)
	close(s.connCh)
	for conn := range s.connCh {
		conn.Close()
	}
	return nil
}

func (s *Server) Serve() {
	cert, err := tls.X509KeyPair(s.conf.certPEMBlock, s.conf.keyPEMBlock)
	if err != nil {
		log.Error(err)
		return
	}
	config := &tls.Config{
		Certificates:             []tls.Certificate{cert},
		MinVersion:               tls.VersionTLS12,
		PreferServerCipherSuites: true,
	}
	addr := net.JoinHostPort(s.conf.Host, strconv.Itoa(s.conf.Port))
	ln, err := tls.Listen("tcp", addr, config)
	if err != nil {
		log.Errorf("yeager proxy failed to listen on %s, error: %s", addr, err)
		return
	}
	defer ln.Close()
	glog.Println("yeager proxy listen on", addr)

	for {
		select {
		case <-s.done:
			return
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
	conn = util.NewMaxIdleConn(conn, 5*time.Minute)
	dstAddr, err := s.handshake(conn)
	if err != nil {
		defer conn.Close()
		log.Errorf("yeager handshake err: %s", err)
		// 如果客户端主动关闭连接或者握手超时，即是客户端缓存的连接因为空闲超时而关闭
		if err == io.EOF && errors.Is(err, os.ErrDeadlineExceeded) {
			return
		}
		// For the anti-detection purpose:
		// All connection without correct structure and password will be redirected to a preset endpoint,
		// so the server behaves exactly the same as that endpoint if a suspicious probe connects.
		// Learning from trojan, https://trojan-gfw.github.io/trojan/protocol
		if s.conf.FallbackUrl != "" {
			err = fallback(conn, s.conf.FallbackUrl)
			if err != nil {
				log.Errorf("fallback err: %s", err)
			}
		}
		return
	}

	select {
	case <-s.done:
		conn.Close()
		return
	case s.connCh <- protocol.NewConn(conn, dstAddr):
	}
}

// 为了降低握手时延，减少一次RTT，yeager出站代理将在建立tls连接后，第一次数据发送时，附带握手信息。
// 当yeager入站代理收到握手信息，如果认证通过，则继续处理，无需回复连接建立；如果认证失败，则关闭连接。
func (s *Server) handshake(conn net.Conn) (dstAddr *protocol.Address, err error) {
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
	if gotUUID != wantUUID {
		return nil, fmt.Errorf("want uuid: %s, got: %s", wantUUID, gotUUID)
	}

	var host string
	switch atyp {
	case addressIPv4:
		var buf [4]byte
		_, err = io.ReadFull(conn, buf[:])
		if err != nil {
			return nil, err
		}
		host = net.IPv4(buf[0], buf[1], buf[2], buf[3]).String()
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
		host = string(bs)
	default:
		return nil, fmt.Errorf("unsupported address type: %x", atyp)
	}

	var bs [2]byte
	_, err = io.ReadFull(conn, bs[:])
	if err != nil {
		return nil, err
	}
	port := binary.BigEndian.Uint16(bs[:])

	dstAddr = protocol.NewAddress(host, int(port))
	return dstAddr, nil
}

// fallback connect to target directly, does not go through the router,
// to keep these code simple and clear
func fallback(conn net.Conn, url string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	return resp.Write(conn)
}

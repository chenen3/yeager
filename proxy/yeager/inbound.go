package yeager

import (
	"crypto/tls"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	glog "log"
	"net"
	"os"
	"strconv"
	"time"

	"github.com/google/uuid"
	"yeager/log"
	"yeager/proxy"
	"yeager/transport/grpc"
	tls2 "yeager/transport/tls"
	"yeager/util"
)

type Server struct {
	conf   *ServerConfig
	connCh chan proxy.Conn
	done   chan struct{}
	ready  chan struct{} // imply that server is ready to accept connection
}

func NewServer(config *ServerConfig) (*Server, error) {
	s := &Server{
		conf:   config,
		connCh: make(chan proxy.Conn, 32),
		done:   make(chan struct{}),
		ready:  make(chan struct{}),
	}
	return s, nil
}

func (s *Server) listen() (net.Listener, error) {
	addr := net.JoinHostPort(s.conf.Host, strconv.Itoa(s.conf.Port))
	cert, err := tls.X509KeyPair(s.conf.TLS.certPEMBlock, s.conf.TLS.keyPEMBlock)
	if err != nil {
		return nil, err
	}

	var lis net.Listener
	tlsConf := &tls.Config{
		Certificates:             []tls.Certificate{cert},
		MinVersion:               tls.VersionTLS12,
		PreferServerCipherSuites: true,
	}
	switch s.conf.Transport {
	case "tls":
		lis, err = tls2.Listen(addr, tlsConf)
	case "grpc":
		lis, err = grpc.Listen(addr, tlsConf)
	default:
		err = errors.New("unsupported transport: " + s.conf.Transport)
	}
	if err != nil {
		return nil, err
	}

	glog.Printf("yeager proxy listen on %s, transport: %s", lis.Addr(), s.conf.Transport)
	return lis, err
}

func (s *Server) Serve() {
	lis, err := s.listen()
	if err != nil {
		panic(err)
	}
	defer lis.Close()

	close(s.ready)
	for {
		select {
		case <-s.done:
			return
		default:
		}

		conn, err := lis.Accept()
		if err != nil {
			log.Error(err)
			continue
		}
		go s.handleConnection(conn)
	}
}

func (s *Server) handleConnection(conn net.Conn) {
	// yeager出站代理和入站代理建立连接后，可能把连接放入连接池，不会立刻发来凭证。
	conn = util.NewMaxIdleConn(conn, 5*time.Minute)
	dstAddr, err := s.parseCredential(conn)
	if err != nil {
		// 客户端主动关闭连接或者握手超时
		if err == io.EOF || errors.Is(err, os.ErrDeadlineExceeded) {
			conn.Close()
			return
		}
		log.Warn("bad credential: " + err.Error())
		// For the anti-detection purpose:
		// All connection without correct structure and password will be redirected to a preset endpoint,
		// so the server behaves exactly the same as that endpoint if a suspicious probe connects.
		// Learning from trojan, https://trojan-gfw.github.io/trojan/protocol
		if s.conf.Fallback.Host == "" {
			conn.Close()
			return
		}
		dstAddr = proxy.NewAddress(s.conf.Fallback.Host, s.conf.Fallback.Port)
	}

	select {
	case <-s.done:
		conn.Close()
		return
	case s.connCh <- proxy.NewConn(conn, dstAddr):
	}
}

// parseCredential 解析凭证，若凭证有效则返回其目的地址
func (s *Server) parseCredential(conn net.Conn) (dstAddr *proxy.Address, err error) {
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
		return nil, fmt.Errorf("%s, UUID: %q", err, uuidBytes)
	}
	wantUUID, err := uuid.Parse(s.conf.UUID)
	if err != nil {
		return nil, err
	}
	if gotUUID != wantUUID {
		return nil, errors.New("mismatch UUID: " + gotUUID.String())
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

	dstAddr = proxy.NewAddress(host, int(port))
	return dstAddr, nil
}

func (s *Server) Accept() <-chan proxy.Conn {
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

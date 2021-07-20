package armin

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

	glog.Printf("armin proxy listen on %s, transport: %s", lis.Addr(), s.conf.Transport)
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
	handshakeTimeout := proxy.HandshakeTimeout
	// 当出站代理使用tls传输方式时，与入站代理建立连接后，
	// 可能把连接放入连接池，不会立刻发来凭证，因此延长超时时间
	if s.conf.Transport == "tls" {
		handshakeTimeout = proxy.IdleConnTimeout
	}
	err := conn.SetDeadline(time.Now().Add(handshakeTimeout))
	if err != nil {
		log.Error("failed to set handshake timeout: " + err.Error())
		conn.Close()
		return
	}

	dstAddr, err := s.parseCredential(conn)
	if err != nil {
		// 客户端主动关闭连接或者握手超时
		if err == io.EOF || errors.Is(err, os.ErrDeadlineExceeded) {
			log.Warn("failed to handshake: " + err.Error())
			conn.Close()
			return
		}
		// For the anti-detection purpose:
		// All connection without correct structure and password will be redirected to a preset endpoint,
		// so the server behaves exactly the same as that endpoint if a suspicious probe connects.
		// Learning from trojan, https://trojan-gfw.github.io/trojan/protocol
		if s.conf.Fallback.Host == "" {
			log.Warn("bad credential: " + err.Error())
			conn.Close()
			return
		}

		dstAddr = proxy.NewAddress(s.conf.Fallback.Host, s.conf.Fallback.Port)
		log.Warnf("bad credential: %s, redirect to %s", err, dstAddr)
	}

	err = conn.SetDeadline(time.Time{})
	if err != nil {
		log.Error("failed to clear handshake timeout: " + err.Error())
		conn.Close()
		return
	}

	newConn := &Conn{
		Conn:        conn,
		dstAddr:     dstAddr,
		idleTimeout: proxy.IdleConnTimeout,
	}
	select {
	case <-s.done:
		newConn.Close()
		return
	case s.connCh <- newConn:
	}
}

// parseCredential 解析凭证，若凭证有效则返回其目的地址
func (s *Server) parseCredential(conn net.Conn) (dstAddr *proxy.Address, err error) {
	/*
		客户端请求格式，仿照socks5协议(以字节为单位):
		VER UUID ATYP DST.ADDR DST.PORT
		1   36   1    动态     2
	*/
	var buf [1 + 36 + 1]byte
	_, err = io.ReadFull(conn, buf[:])
	if err != nil {
		return nil, err
	}

	version, uuidBytes, atyp := buf[0], buf[1:37], buf[37]
	// keep version number for backward compatibility
	_ = version
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

// TODO: 可以改为在外边注册注册handler，然后所有连接都在此server中处理，不需要传给外面，减少堆内存分配
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

package armin

import (
	"context"
	"crypto/tls"
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
	"yeager/transport/grpc"

	"github.com/caddyserver/certmagic"
	"github.com/google/uuid"
)

type Server struct {
	ctx    context.Context
	cancel context.CancelFunc
	conf   *config.ArminServerConfig
	lis    net.Listener
	wg     sync.WaitGroup // counts active Serve goroutines for graceful close

	ready chan struct{} // imply that server is ready to accept connection, testing only
}

func NewServer(config *config.ArminServerConfig) *Server {
	ctx, cancel := context.WithCancel(context.Background())
	return &Server{
		conf:   config,
		ready:  make(chan struct{}),
		ctx:    ctx,
		cancel: cancel,
	}
}

func makeTLSConfig(ac *config.ArminServerConfig) (*tls.Config, error) {
	var err error
	var tlsConf *tls.Config
	if ac.ACME.Domain != "" {
		// manage certificate automatically
		certmagic.DefaultACME.Agreed = true
		certmagic.DefaultACME.Email = ac.ACME.Email
		if ac.ACME.StagingCA {
			certmagic.DefaultACME.CA = certmagic.LetsEncryptStagingCA
		} else {
			certmagic.DefaultACME.CA = certmagic.LetsEncryptProductionCA
		}

		tlsConf, err = certmagic.TLS([]string{ac.ACME.Domain})
		if err != nil {
			return nil, err
		}

	} else {
		// manage certificate manually
		var cert tls.Certificate
		if ac.CertFile != "" && ac.KeyFile != "" {
			cert, err = tls.LoadX509KeyPair(ac.CertFile, ac.KeyFile)
		} else {
			cert, err = tls.X509KeyPair(ac.CertPEMBlock, ac.KeyPEMBlock)
		}
		if err != nil {
			return nil, err
		}

		tlsConf = &tls.Config{Certificates: []tls.Certificate{cert}}
	}

	tlsConf.MinVersion = tls.VersionTLS13
	return tlsConf, nil
}

func (s *Server) listen() (net.Listener, error) {
	var lis net.Listener
	var err error
	switch s.conf.Transport {
	case "tcp":
		lis, err = net.Listen("tcp", s.conf.Address)
	case "tls":
		tlsConf, err := makeTLSConfig(s.conf)
		if err != nil {
			return nil, err
		}
		lis, err = tls.Listen("tcp", s.conf.Address, tlsConf)
	case "grpc":
		if s.conf.Plaintext {
			lis, err = grpc.Listen(s.conf.Address, nil)
		} else {
			tlsConf, err := makeTLSConfig(s.conf)
			if err != nil {
				return nil, err
			}
			lis, err = grpc.Listen(s.conf.Address, tlsConf)
		}
	default:
		err = errors.New("unsupported transport: " + s.conf.Transport)
	}
	if err != nil {
		return nil, err
	}

	log.Infof("armin proxy listen on %s, transport: %s", lis.Addr(), s.conf.Transport)
	return lis, err
}

func (s *Server) ListenAndServe(handle proxy.Handler) error {
	lis, err := s.listen()
	if err != nil {
		return err
	}
	s.lis = lis

	close(s.ready)
	for {
		select {
		case <-s.ctx.Done():
			return nil
		default:
		}

		conn, err := lis.Accept()
		if err != nil {
			log.Warn(err)
			continue
		}

		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			dstAddr, err := s.parseCredential(conn)
			if err != nil {
				log.Warn("failed to parse credential: " + err.Error())
				conn.Close()
				return
			}

			newConn := &Conn{
				Conn:        conn,
				idleTimeout: proxy.IdleConnTimeout,
			}
			handle(s.ctx, newConn, dstAddr)
		}()
	}
}

func (s *Server) Close() error {
	defer s.wg.Wait()
	s.cancel()
	return s.lis.Close()
}

// parseCredential 解析凭证，若凭证有效则返回其目的地址
func (s *Server) parseCredential(conn net.Conn) (dstAddr *proxy.Address, err error) {
	timeout := proxy.HandshakeTimeout
	// 当出站代理使用tls传输方式时，与入站代理建立连接后，
	// 可能把连接放入连接池，不会立刻发来凭证，因此延长超时时间
	if s.conf.Transport == "tls" {
		timeout = proxy.IdleConnTimeout
	}
	err = conn.SetDeadline(time.Now().Add(timeout))
	if err != nil {
		return
	}
	defer func() {
		er := conn.SetDeadline(time.Time{})
		if er != nil && err == nil {
			err = er
		}
	}()

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

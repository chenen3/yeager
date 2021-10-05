package yeager

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/chenen3/yeager/config"
	"github.com/chenen3/yeager/proxy/common"
	"github.com/chenen3/yeager/transport/grpc"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"golang.org/x/crypto/acme/autocert"
)

const certDir = "/usr/local/etc/yeager/golang-autocert"

type Server struct {
	ctx    context.Context
	cancel context.CancelFunc
	conf   *config.YeagerServer
	lis    net.Listener
	wg     sync.WaitGroup // counts active Serve goroutines for graceful close

	ready chan struct{} // imply that server is ready to accept connection, testing only
}

func NewServer(conf *config.YeagerServer) (*Server, error) {
	if conf == nil || conf.Listen == "" {
		return nil, errors.New("config missing listening address")
	}

	ctx, cancel := context.WithCancel(context.Background())
	return &Server{
		conf:   conf,
		ready:  make(chan struct{}),
		ctx:    ctx,
		cancel: cancel,
	}, nil
}

func makeServerTLSConfig(conf *config.YeagerServer) (*tls.Config, error) {
	tlsConf := new(tls.Config)
	switch conf.Security {
	case config.TLS:
		var cert tls.Certificate
		var err error
		if len(conf.TLS.CertPEM) != 0 && len(conf.TLS.KeyPEM) != 0 {
			cert, err = tls.X509KeyPair(conf.TLS.CertPEM, conf.TLS.KeyPEM)
			if err != nil {
				return nil, errors.New("failed to make TLS config: " + err.Error())
			}
		} else {
			cert, err = tls.LoadX509KeyPair(conf.TLS.CertFile, conf.TLS.KeyFile)
			if err != nil {
				return nil, errors.New("failed to make TLS config: " + err.Error())
			}
		}
		tlsConf = &tls.Config{Certificates: []tls.Certificate{cert}}

	case config.TLSAcme:
		if conf.ACME.Domain == "" {
			return nil, errors.New("domain required")
		}
		m := &autocert.Manager{
			Cache:      autocert.DirCache(certDir),
			Prompt:     autocert.AcceptTOS,
			HostPolicy: autocert.HostWhitelist(conf.ACME.Domain),
		}
		s := &http.Server{
			Addr:      ":https",
			TLSConfig: m.TLSConfig(),
		}
		go func() {
			zap.S().Error(s.ListenAndServeTLS("", ""))
		}()
		tlsConf = m.TLSConfig()

	case config.TLSMutual:
		var cert tls.Certificate
		var err error
		if len(conf.MTLS.CertPEM) != 0 && len(conf.MTLS.KeyPEM) != 0 {
			cert, err = tls.X509KeyPair(conf.MTLS.CertPEM, conf.MTLS.KeyPEM)
			if err != nil {
				return nil, errors.New("failed to make TLS config: " + err.Error())
			}
		} else if conf.MTLS.CertFile != "" && conf.MTLS.KeyFile != "" {
			cert, err = tls.LoadX509KeyPair(conf.MTLS.CertFile, conf.MTLS.KeyFile)
			if err != nil {
				return nil, errors.New("failed to make TLS config: " + err.Error())
			}
		} else {
			return nil, errors.New("certificate and key required")
		}

		tlsConf = &tls.Config{Certificates: []tls.Certificate{cert}}
		if len(conf.MTLS.ClientCA) != 0 {
			pool := x509.NewCertPool()
			ok := pool.AppendCertsFromPEM(conf.MTLS.ClientCA)
			if !ok {
				return nil, errors.New("failed to parse root certificate")
			}
			tlsConf.ClientCAs = pool
			tlsConf.ClientAuth = tls.RequireAndVerifyClientCert
		} else if conf.MTLS.ClientCAFile != "" {
			ca, err := os.ReadFile(conf.MTLS.ClientCAFile)
			if err != nil {
				return nil, err
			}
			pool := x509.NewCertPool()
			ok := pool.AppendCertsFromPEM(ca)
			if !ok {
				return nil, errors.New("failed to parse root certificate")
			}
			tlsConf.ClientCAs = pool
			tlsConf.ClientAuth = tls.RequireAndVerifyClientCert
		} else {
			return nil, errors.New("certificate and key required")
		}
	default:
		return nil, fmt.Errorf("unsupported security: %s", conf.Security)
	}

	tlsConf.MinVersion = tls.VersionTLS13
	return tlsConf, nil
}

func (s *Server) listen() (net.Listener, error) {
	var lis net.Listener
	var err error
	switch s.conf.Transport {
	case config.TransTCP:
		if s.conf.Security == config.NoSecurity {
			lis, err = net.Listen("tcp", s.conf.Listen)
			if err != nil {
				return nil, err
			}
		} else {
			var tlsConf *tls.Config
			tlsConf, err = makeServerTLSConfig(s.conf)
			if err != nil {
				return nil, err
			}
			lis, err = tls.Listen("tcp", s.conf.Listen, tlsConf)
			if err != nil {
				return nil, err
			}
		}
	case config.TransGRPC:
		if s.conf.Security == config.NoSecurity {
			lis, err = grpc.Listen(s.conf.Listen, nil)
			if err != nil {
				return nil, err
			}
		} else {
			// var tlsConf *tls.Config
			tlsConf, err := makeServerTLSConfig(s.conf)
			if err != nil {
				return nil, err
			}
			lis, err = grpc.Listen(s.conf.Listen, tlsConf)
			if err != nil {
				return nil, err
			}
		}
	default:
		return nil, fmt.Errorf("unsupported transport: %s", s.conf.Transport)
	}

	zap.S().Infof("yeager proxy listening %s", lis.Addr())
	return lis, nil
}

func (s *Server) ListenAndServe(handle func(ctx context.Context, conn net.Conn, addr string)) error {
	lis, err := s.listen()
	if err != nil {
		return err
	}
	s.lis = lis

	close(s.ready)
	for {
		conn, err := lis.Accept()
		if err != nil {
			select {
			case <-s.ctx.Done():
				return nil
			default:
			}
			zap.S().Warn(err)
			continue
		}

		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			dstAddr, err := s.parseMetaData(conn)
			if err != nil {
				zap.S().Warn("failed to parse metadata: " + err.Error())
				conn.Close()
				return
			}

			newConn := &Conn{
				Conn:    conn,
				maxIdle: common.MaxConnectionIdle, // FIXME
			}
			handle(s.ctx, newConn, dstAddr)
		}()
	}
}

func (s *Server) Close() error {
	defer s.wg.Wait()
	s.cancel()
	if s.lis != nil {
		return s.lis.Close()
	}
	return nil
}

// parseMetaData 解析元数据，若凭证有效则返回其目的地址
func (s *Server) parseMetaData(conn net.Conn) (addr string, err error) {
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

	/*
		客户端请求格式，仿照socks5协议(以字节为单位):
		VER UUID ATYP DST.ADDR DST.PORT
		1   36   1    动态     2
	*/
	var buf [1 + 36 + 1]byte
	_, err = io.ReadFull(conn, buf[:])
	if err != nil {
		return "", err
	}

	version, uuidBytes, atyp := buf[0], buf[1:37], buf[37]
	// keep version number for backward compatibility
	_ = version

	// when use mutual authentication, UUID is no longer needed
	if s.conf.Security != config.TLSMutual {
		gotUUID, err := uuid.ParseBytes(uuidBytes)
		if err != nil {
			return "", fmt.Errorf("%s, UUID: %q", err, uuidBytes)
		}
		wantUUID, err := uuid.Parse(s.conf.UUID)
		if err != nil {
			return "", err
		}
		if gotUUID != wantUUID {
			return "", errors.New("mismatch UUID: " + gotUUID.String())
		}
	}

	var host string
	switch atyp {
	case addressIPv4:
		var buf [4]byte
		_, err = io.ReadFull(conn, buf[:])
		if err != nil {
			return "", err
		}
		host = net.IPv4(buf[0], buf[1], buf[2], buf[3]).String()
	case addressDomain:
		var buf [1]byte
		_, err = io.ReadFull(conn, buf[:])
		if err != nil {
			return "", err
		}
		length := buf[0]

		bs := make([]byte, length)
		_, err := io.ReadFull(conn, bs)
		if err != nil {
			return "", err
		}
		host = string(bs)
	default:
		return "", fmt.Errorf("unsupported address type: %x", atyp)
	}

	var bs [2]byte
	_, err = io.ReadFull(conn, bs[:])
	if err != nil {
		return "", err
	}

	port := binary.BigEndian.Uint16(bs[:])
	addr = net.JoinHostPort(host, strconv.Itoa(int(port)))
	return addr, nil
}

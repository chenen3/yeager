package tunnel

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/chenen3/yeager/config"
	"github.com/chenen3/yeager/tunnel/grpc"
	"github.com/chenen3/yeager/tunnel/quic"
	"github.com/chenen3/yeager/util"
)

// Server implements the Inbounder interface
type Server struct {
	conf       *config.YeagerServer
	lis        net.Listener
	handleConn func(ctx context.Context, c net.Conn, addr string)
	wg         sync.WaitGroup
	ctx        context.Context
	cancel     context.CancelFunc
	ready      chan struct{}
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

func (s *Server) RegisterHandler(handleConn func(ctx context.Context, c net.Conn, addr string)) {
	s.handleConn = handleConn
}

// return tls.Config for mutual TLS usage
func makeServerTLSConfig(conf *config.YeagerServer) (*tls.Config, error) {
	tlsConf := &tls.Config{
		MinVersion: tls.VersionTLS13,
	}
	if len(conf.TLS.CertPEM) != 0 && len(conf.TLS.KeyPEM) != 0 {
		cert, err := tls.X509KeyPair([]byte(conf.TLS.CertPEM), []byte(conf.TLS.KeyPEM))
		if err != nil {
			return nil, errors.New("parse cert pem: " + err.Error())
		}
		tlsConf.Certificates = []tls.Certificate{cert}
	} else if conf.TLS.CertFile != "" && conf.TLS.KeyFile != "" {
		cert, err := tls.LoadX509KeyPair(conf.TLS.CertFile, conf.TLS.KeyFile)
		if err != nil {
			return nil, errors.New("parse cert file: " + err.Error())
		}
		tlsConf.Certificates = []tls.Certificate{cert}
	} else {
		return nil, errors.New("certificate and key required")
	}

	if len(conf.TLS.CAPEM) != 0 {
		pool := x509.NewCertPool()
		ok := pool.AppendCertsFromPEM([]byte(conf.TLS.CAPEM))
		if !ok {
			return nil, errors.New("failed to parse root cert pem")
		}
		tlsConf.ClientCAs = pool
		tlsConf.ClientAuth = tls.RequireAndVerifyClientCert
	} else if conf.TLS.CAFile != "" {
		ca, err := os.ReadFile(conf.TLS.CAFile)
		if err != nil {
			return nil, err
		}
		pool := x509.NewCertPool()
		ok := pool.AppendCertsFromPEM(ca)
		if !ok {
			return nil, errors.New("failed to parse root cert file")
		}
		tlsConf.ClientCAs = pool
		tlsConf.ClientAuth = tls.RequireAndVerifyClientCert
	} else {
		return nil, errors.New("certificate and key required")
	}

	return tlsConf, nil
}

func (s *Server) listen() (net.Listener, error) {
	switch s.conf.Transport {
	case config.TransTCP:
		return net.Listen("tcp", s.conf.Listen)
	case config.TransGRPC:
		tlsConf, err := makeServerTLSConfig(s.conf)
		if err != nil {
			return nil, err
		}
		return grpc.Listen(s.conf.Listen, tlsConf)
	case config.TransQUIC:
		tlsConf, err := makeServerTLSConfig(s.conf)
		if err != nil {
			return nil, err
		}
		return quic.Listen(s.conf.Listen, tlsConf)
	default:
		return nil, fmt.Errorf("unknown transport: %s", s.conf.Transport)
	}
}

func (s *Server) ListenAndServe() error {
	lis, err := s.listen()
	if err != nil {
		return fmt.Errorf("tunnel listen: %s", err)
	}
	s.lis = lis
	log.Printf("tunnel listening %s %s", s.conf.Transport, lis.Addr())

	close(s.ready)
	for {
		conn, err := lis.Accept()
		if err != nil {
			return err
		}

		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			defer conn.Close()
			conn.SetReadDeadline(time.Now().Add(util.HandshakeTimeout))
			dstAddr, err := parseHeader(conn)
			if err != nil {
				log.Printf("failed to parse header, peer: %s, err: %s", conn.RemoteAddr(), err)
				if _, err = io.Copy(io.Discard, conn); err != nil {
					log.Printf("failed to drain bad connection: %s", err)
				}
				return
			}
			conn.SetReadDeadline(time.Time{})
			s.handleConn(s.ctx, conn, dstAddr)
		}()
	}
}

func (s *Server) Close() error {
	s.cancel()
	var err error
	if s.lis != nil {
		err = s.lis.Close()
	}
	return err
}

// Shutdown gracefully shuts down the server,
// it works by first closing listener,
// then wait for all connection to close
func (s *Server) Shutdown() error {
	s.cancel()
	var err error
	if s.lis != nil {
		err = s.lis.Close()
	}
	s.wg.Wait()
	return err
}

// parseHeader 读取 header, 解析其目的地址
func parseHeader(r io.Reader) (addr string, err error) {
	/*
		客户端请求格式，仿照socks5协议(以字节为单位):
		VER ATYP DST.ADDR DST.PORT
		1   1    动态     2
	*/
	b := make([]byte, 1+maxAddrLen)
	if _, err = io.ReadFull(r, b[:2]); err != nil {
		return "", err
	}

	atyp := b[1]
	var host string
	switch atyp {
	case addrIPv4:
		if _, err = io.ReadFull(r, b[:net.IPv4len]); err != nil {
			return "", err
		}
		host = net.IPv4(b[0], b[1], b[2], b[3]).String()
	case addrDomain:
		if _, err = io.ReadFull(r, b[:1]); err != nil {
			return "", err
		}
		domainLen := b[0]
		if _, err = io.ReadFull(r, b[:domainLen]); err != nil {
			return "", err
		}
		host = string(b[:domainLen])
	case addrIPv6:
		if _, err = io.ReadFull(r, b[:net.IPv6len]); err != nil {
			return "", err
		}
		ipv6 := make(net.IP, net.IPv6len)
		copy(ipv6, b[:net.IPv6len])
		host = ipv6.String()
	default:
		return "", fmt.Errorf("unsupported address type: %x", atyp)
	}

	if _, err = io.ReadFull(r, b[:2]); err != nil {
		return "", err
	}
	port := binary.BigEndian.Uint16(b[:2])
	addr = net.JoinHostPort(host, strconv.Itoa(int(port)))
	return addr, nil
}

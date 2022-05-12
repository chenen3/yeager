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
	conf    *config.YeagerServer
	lis     net.Listener
	handler func(ctx context.Context, c net.Conn, addr string)

	wg     sync.WaitGroup
	ctx    context.Context
	cancel context.CancelFunc
	ready  chan struct{}
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

func (s *Server) Handle(handler func(ctx context.Context, c net.Conn, addr string)) {
	s.handler = handler
}

// return tls.Config for mutual TLS usage
func makeServerTLSConfig(conf *config.YeagerServer) (*tls.Config, error) {
	tlsConf := &tls.Config{
		MinVersion: tls.VersionTLS13,
	}
	if len(conf.MutualTLS.CertPEM) != 0 && len(conf.MutualTLS.KeyPEM) != 0 {
		cert, err := tls.X509KeyPair(conf.MutualTLS.CertPEM, conf.MutualTLS.KeyPEM)
		if err != nil {
			return nil, errors.New("parse cert pem: " + err.Error())
		}
		tlsConf.Certificates = []tls.Certificate{cert}
	} else if conf.MutualTLS.CertFile != "" && conf.MutualTLS.KeyFile != "" {
		cert, err := tls.LoadX509KeyPair(conf.MutualTLS.CertFile, conf.MutualTLS.KeyFile)
		if err != nil {
			return nil, errors.New("parse cert file: " + err.Error())
		}
		tlsConf.Certificates = []tls.Certificate{cert}
	} else {
		return nil, errors.New("certificate and key required")
	}

	if len(conf.MutualTLS.CAPEM) != 0 {
		pool := x509.NewCertPool()
		ok := pool.AppendCertsFromPEM(conf.MutualTLS.CAPEM)
		if !ok {
			return nil, errors.New("failed to parse root cert pem")
		}
		tlsConf.ClientCAs = pool
		tlsConf.ClientAuth = tls.RequireAndVerifyClientCert
	} else if conf.MutualTLS.CAFile != "" {
		ca, err := os.ReadFile(conf.MutualTLS.CAFile)
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
	var lis net.Listener
	var err error
	switch s.conf.Transport {
	case config.TransTCP:
		if lis, err = net.Listen("tcp", s.conf.Listen); err != nil {
			return nil, err
		}
	case config.TransGRPC:
		tlsConf, err := makeServerTLSConfig(s.conf)
		if err != nil {
			return nil, err
		}
		if lis, err = grpc.Listen(s.conf.Listen, tlsConf); err != nil {
			return nil, err
		}
	case config.TransQUIC:
		tlsConf, err := makeServerTLSConfig(s.conf)
		if err != nil {
			return nil, err
		}
		if lis, err = quic.Listen(s.conf.Listen, tlsConf); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unsupported transport: %s", s.conf.Transport)
	}

	return lis, nil
}

func (s *Server) ListenAndServe() error {
	lis, err := s.listen()
	if err != nil {
		return fmt.Errorf("tunnel listen: %s", err)
	}
	s.lis = lis
	log.Printf("tunnel listening %s", lis.Addr())

	close(s.ready)
	for {
		conn, err := lis.Accept()
		if err != nil {
			select {
			case <-s.ctx.Done():
				return nil
			default:
			}
			log.Printf("failed to accpet conn: %s", err)
			continue
		}

		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			defer conn.Close()
			err = conn.SetReadDeadline(time.Now().Add(util.HandshakeTimeout))
			if err != nil {
				return
			}
			dstAddr, err := parseHeader(conn)
			if err != nil {
				log.Printf("failed to parse header, peer: %s, err: %s", conn.RemoteAddr(), err)
				if _, err = io.Copy(io.Discard, conn); err != nil {
					log.Printf("failed to drain bad connection: %s", err)
				}
				return
			}
			err = conn.SetReadDeadline(time.Time{})
			if err != nil {
				return
			}

			s.handler(s.ctx, conn, dstAddr)
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

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"

	"github.com/chenen3/yeager/cert"
	"github.com/chenen3/yeager/logger"
	"github.com/chenen3/yeager/proxy"
	"github.com/chenen3/yeager/transport"
	"github.com/chenen3/yeager/transport/grpc"
	"github.com/chenen3/yeager/transport/http2"
)

// StartServices starts services with the given config,
// any started service will be return as io.Closer for future stopping
func StartServices(conf Config) ([]io.Closer, error) {
	if len(conf.Listen) == 0 && conf.Proxy.Address == "" {
		return nil, errors.New("no proxy client nor server specified in config")
	}

	var services []io.Closer
	if conf.Proxy.Address != "" {
		dialer, err := newTransportDialer(conf.Proxy)
		if err != nil {
			return nil, err
		}
		services = append(services, dialer)

		if conf.ListenHTTP != "" {
			s := &http.Server{
				Addr:    conf.ListenHTTP,
				Handler: proxy.NewHTTPHandler(dialer),
			}
			go func() {
				err := s.ListenAndServe()
				if err != nil && err != http.ErrServerClosed {
					logger.Error.Printf("serve http proxy: %s", err)
				}
			}()
			services = append(services, s)
		}

		if conf.ListenSOCKS != "" {
			lis, err := net.Listen("tcp", conf.ListenSOCKS)
			if err != nil {
				return nil, err
			}
			ss := proxy.NewSOCKS5Server(dialer)
			go func() {
				err := ss.Serve(lis)
				if err != nil {
					logger.Error.Printf("serve socks proxy: %s", err)
				}
			}()
			services = append(services, ss)
		}
	}

	for _, sc := range conf.Listen {
		sc := sc
		certPEM, err := sc.GetCertPEM()
		if err != nil {
			return nil, fmt.Errorf("read certificate: %s", err)
		}
		keyPEM, err := sc.GetKeyPEM()
		if err != nil {
			return nil, fmt.Errorf("read key: %s", err)
		}
		caPEM, err := sc.GetCAPEM()
		if err != nil {
			return nil, fmt.Errorf("read CA: %s", err)
		}
		tlsConf, err := cert.ServerTLSConfig(caPEM, certPEM, keyPEM)
		if err != nil {
			return nil, err
		}

		switch sc.Proto {
		case ProtoGRPC:
			lis, err := net.Listen("tcp", sc.Address)
			if err != nil {
				return nil, err
			}
			var s grpc.Server
			go func() {
				if err := s.Serve(lis, tlsConf); err != nil {
					logger.Error.Printf("transport server: %s", err)
				}
			}()
			services = append(services, &s)
		case ProtoHTTP2:
			var s http2.Server
			go func() {
				if err := s.Serve(sc.Address, tlsConf, sc.Username, sc.Password); err != nil {
					logger.Error.Printf("transport server: %s", err)
				}
			}()
			services = append(services, &s)
		}
	}
	return services, nil
}

func closeAll(services []io.Closer) {
	for _, s := range services {
		if err := s.Close(); err != nil {
			logger.Error.Print(err)
		}
	}
}

// wrapper for transport.Dialer so that private requests
// do not go through the transport server by default
type transDialer struct {
	dialer       transport.Dialer
	allowPrivate bool
}

func newTransportDialer(cc ServerConfig) (*transDialer, error) {
	hasAuth := cc.Username != "" && cc.Password != ""
	certPEM, err := cc.GetCertPEM()
	if err != nil && !hasAuth {
		return nil, fmt.Errorf("read certificate: %s", err)
	}
	keyPEM, err := cc.GetKeyPEM()
	if err != nil && !hasAuth {
		return nil, fmt.Errorf("read key: %s", err)
	}
	caPEM, err := cc.GetCAPEM()
	if err != nil && !hasAuth {
		return nil, fmt.Errorf("read CA: %s", err)
	}
	tlsConf, err := cert.ClientTLSConfig(caPEM, certPEM, keyPEM)
	if err != nil && !hasAuth {
		return nil, fmt.Errorf("make tls conf: %s", err)
	}

	var d transport.Dialer
	switch cc.Proto {
	case ProtoGRPC:
		d = grpc.NewDialer(cc.Address, tlsConf)
	case ProtoHTTP2:
		d = http2.NewDialer(cc.Address, tlsConf, cc.Username, cc.Password)
	default:
		return nil, errors.New("unsupported proxy protocol: " + cc.Proto)
	}
	return &transDialer{dialer: d, allowPrivate: cc.allowPrivate}, nil
}

func private(host string) bool {
	if host == "localhost" {
		return true
	}
	if ip := net.ParseIP(host); ip != nil && (ip.IsLoopback() || ip.IsPrivate()) {
		return true
	}
	return false
}

func (d *transDialer) Dial(ctx context.Context, address string) (transport.Stream, error) {
	logger.Info.Printf("connect to %s", address)
	if !d.allowPrivate {
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		if private(host) {
			var d net.Dialer
			conn, err := d.DialContext(ctx, "tcp", address)
			if err != nil {
				return nil, err
			}
			tcpConn, _ := conn.(*net.TCPConn)
			return tcpConn, nil
		}
	}
	return d.dialer.Dial(ctx, address)
}

func (d *transDialer) Close() error {
	c, ok := d.dialer.(io.Closer)
	if !ok {
		return nil
	}
	return c.Close()
}

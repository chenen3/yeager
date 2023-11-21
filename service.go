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

// start the service specified by config.
// The caller should call stop when finished.
func start(cfg Config) (stop func(), err error) {
	var toClose []io.Closer
	stop = func() {
		for _, c := range toClose {
			e := c.Close()
			if e != nil {
				logger.Error.Print(e)
			}
		}
	}
	defer func() {
		if err != nil {
			stop()
		}
	}()

	if len(cfg.Listen) == 0 && cfg.Proxy.Address == "" {
		return nil, errors.New("missing client or server config")
	}

	if cfg.Proxy.Address != "" {
		dialer, err := newStreamDialer(cfg.Proxy)
		if err != nil {
			return nil, err
		}
		if v, ok := dialer.(io.Closer); ok {
			toClose = append(toClose, v)
		}
		if !cfg.Proxy.allowPrivate {
			dialer = bypassPrivate(dialer)
		}

		if cfg.ListenHTTP != "" {
			listener, err := net.Listen("tcp", cfg.ListenHTTP)
			if err != nil {
				return nil, err
			}
			s := &http.Server{Handler: proxy.NewHTTPHandler(dialer)}
			go func() {
				err := s.Serve(listener)
				if err != nil && err != http.ErrServerClosed {
					logger.Error.Printf("serve http proxy: %s", err)
				}
			}()
			toClose = append(toClose, s)
		}

		if cfg.ListenSOCKS != "" {
			listener, err := net.Listen("tcp", cfg.ListenSOCKS)
			if err != nil {
				return nil, err
			}
			ss := proxy.NewSOCKS5Server(dialer)
			go func() {
				err := ss.Serve(listener)
				if err != nil {
					logger.Error.Printf("serve socks proxy: %s", err)
				}
			}()
			toClose = append(toClose, ss)
		}
	}

	for _, sc := range cfg.Listen {
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
					logger.Error.Printf("grpc serve: %s", err)
				}
			}()
			toClose = append(toClose, &s)
		case ProtoHTTP2:
			s, err := http2.StartServer(sc.Address, tlsConf, sc.Username, sc.Password)
			if err != nil {
				logger.Error.Printf("http2 serve: %s", err)
			}
			toClose = append(toClose, s)
		}
	}
	return stop, nil
}

func newStreamDialer(cc ServerConfig) (transport.StreamDialer, error) {
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

	var d transport.StreamDialer
	switch cc.Proto {
	case ProtoGRPC:
		d = grpc.NewStreamDialer(cc.Address, tlsConf)
	case ProtoHTTP2:
		d = http2.NewStreamDialer(cc.Address, tlsConf, cc.Username, cc.Password)
	default:
		return nil, errors.New("unsupported protocol: " + cc.Proto)
	}
	return d, nil
}

type dialerWithPrivate struct {
	transport.StreamDialer
	direct transport.TCPStreamDialer
}

// wraps a StreamDialer and bypass private host,
// the returned dialer will connect directly to
// private host instead of using the StreamDialer
func bypassPrivate(d transport.StreamDialer) transport.StreamDialer {
	return &dialerWithPrivate{StreamDialer: d}
}

func (d dialerWithPrivate) Dial(ctx context.Context, address string) (transport.Stream, error) {
	private := func(host string) bool {
		if host == "localhost" {
			return true
		}
		if ip := net.ParseIP(host); ip != nil && (ip.IsLoopback() || ip.IsPrivate()) {
			return true
		}
		return false
	}

	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	if private(host) {
		return d.direct.Dial(ctx, address)
	}
	return d.StreamDialer.Dial(ctx, address)
}

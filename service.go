package main

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

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
	var onStop []func() error
	defer func() {
		if err != nil {
			for _, f := range onStop {
				if e := f(); e != nil {
					logger.Error.Print(e)
				}
			}
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
			onStop = append(onStop, v.Close)
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
			onStop = append(onStop, s.Close)
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
			onStop = append(onStop, ss.Close)
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
			s, err := grpc.NewServer(sc.Address, tlsConf)
			if err != nil {
				return nil, err
			}
			onStop = append(onStop, func() error {
				s.Stop()
				return nil
			})
		case ProtoHTTP2:
			s, err := http2.NewServer(sc.Address, tlsConf, sc.Username, sc.Password)
			if err != nil {
				return nil, err
			}
			onStop = append(onStop, s.Close)
		}
	}

	stop = func() {
		for _, f := range onStop {
			if e := f(); e != nil {
				logger.Error.Print(e)
			}
		}
	}
	return stop, nil
}

func newStreamDialer(c TransportConfig) (transport.StreamDialer, error) {
	var tlsConf *tls.Config
	if c.Proto != ProtoHTTP2 || c.Username == "" || c.Password == "" {
		certPEM, err := c.GetCertPEM()
		if err != nil {
			return nil, fmt.Errorf("read certificate: %s", err)
		}
		keyPEM, err := c.GetKeyPEM()
		if err != nil {
			return nil, fmt.Errorf("read key: %s", err)
		}
		caPEM, err := c.GetCAPEM()
		if err != nil {
			return nil, fmt.Errorf("read CA: %s", err)
		}
		tlsConf, err = cert.ClientTLSConfig(caPEM, certPEM, keyPEM)
		if err != nil {
			return nil, fmt.Errorf("make tls conf: %s", err)
		}
	}

	var d transport.StreamDialer
	switch c.Proto {
	case ProtoGRPC:
		d = grpc.NewStreamDialer(c.Address, tlsConf)
	case ProtoHTTP2:
		d = http2.NewStreamDialer(c.Address, tlsConf, c.Username, c.Password)
	default:
		return nil, errors.New("unsupported transport protocol: " + c.Proto)
	}
	if !c.allowPrivate {
		d = directPrivate(d)
	}
	return d, nil
}

type dialerWithPrivate struct {
	transport.StreamDialer
	direct transport.TCPStreamDialer
}

// For private address, the returned dialer connects directly to it
// rather than through the transport
func directPrivate(d transport.StreamDialer) transport.StreamDialer {
	return &dialerWithPrivate{StreamDialer: d}
}

func (d dialerWithPrivate) Dial(ctx context.Context, address string) (stream transport.Stream, err error) {
	start := time.Now()
	defer func() {
		if err == nil {
			logger.Debug.Printf("connected to %s, timed: %dms", address, time.Since(start).Milliseconds())
		}
	}()

	isPrivate := func(host string) bool {
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
	if isPrivate(host) {
		return d.direct.Dial(ctx, address)
	}
	return d.StreamDialer.Dial(ctx, address)
}

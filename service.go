package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"

	"github.com/chenen3/yeager/cert"
	"github.com/chenen3/yeager/proxy"
	"github.com/chenen3/yeager/tunnel"
	"github.com/chenen3/yeager/tunnel/grpc"
	"github.com/chenen3/yeager/tunnel/http2"
	"github.com/chenen3/yeager/tunnel/quic"
)

// StartServices starts services with the given config,
// any started service will be return as io.Closer for future stopping
func StartServices(conf Config) ([]io.Closer, error) {
	if len(conf.Listen) == 0 && conf.Proxy.Address == "" {
		return nil, errors.New("no proxy client nor server specified in config")
	}

	var services []io.Closer
	if conf.Proxy.Address != "" {
		pd, err := newProxyDialer(conf.Proxy)
		if err != nil {
			return nil, err
		}
		services = append(services, pd)

		if conf.ListenHTTP != "" {
			lis, err := net.Listen("tcp", conf.ListenHTTP)
			if err != nil {
				return nil, err
			}
			hs := new(proxy.HTTPServer)
			go func() {
				slog.Info("listen http " + conf.ListenHTTP)
				if err := hs.Serve(lis, pd.DialContext); err != nil {
					slog.Error("failed to serve http proxy: " + err.Error())
				}
			}()
			services = append(services, hs)
		}

		if conf.ListenSOCKS != "" {
			lis, err := net.Listen("tcp", conf.ListenSOCKS)
			if err != nil {
				return nil, err
			}
			ss := new(proxy.SOCKSServer)
			go func() {
				slog.Info("listen socks " + conf.ListenSOCKS)
				if err := ss.Serve(lis, pd.DialContext); err != nil {
					slog.Error("failed to serve socks proxy: " + err.Error())
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
		tlsConf, err := cert.MakeServerTLSConfig(caPEM, certPEM, keyPEM)
		if err != nil {
			return nil, err
		}

		switch sc.Proto {
		case ProtoGRPC:
			lis, err := net.Listen("tcp", sc.Address)
			if err != nil {
				return nil, err
			}
			var s grpc.TunnelServer
			go func() {
				if err := s.Serve(lis, tlsConf); err != nil {
					slog.Error("start tunnel: "+err.Error(), "proto", sc.Proto)
				}
			}()
			services = append(services, &s)
		case ProtoQUIC:
			var s quic.TunnelServer
			go func() {
				if err := s.Serve(sc.Address, tlsConf); err != nil {
					slog.Error("start tunnel: "+err.Error(), "proto", sc.Proto)
				}
			}()
			services = append(services, &s)
		case ProtoHTTP2:
			var s http2.TunnelServer
			go func() {
				if err := s.Serve(sc.Address, tlsConf, sc.Username, sc.Password); err != nil {
					slog.Error("start tunnel: "+err.Error(), "proto", sc.Proto)
				}
			}()
			services = append(services, &s)
		}
		slog.Info(fmt.Sprintf("listen %s %s", sc.Proto, sc.Address))
	}
	return services, nil
}

func closeAll(services []io.Closer) {
	for _, s := range services {
		if err := s.Close(); err != nil {
			slog.Error(err.Error())
		}
	}
}

type proxyDialer struct {
	dialer       tunnel.Dialer
	allowPrivate bool
}

func newProxyDialer(cc ServerConfig) (*proxyDialer, error) {
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
	tlsConf, err := cert.MakeClientTLSConfig(caPEM, certPEM, keyPEM)
	if err != nil && !hasAuth {
		return nil, fmt.Errorf("make tls conf: %s", err)
	}

	var d tunnel.Dialer
	switch cc.Proto {
	case ProtoGRPC:
		d = grpc.NewTunnelClient(cc.Address, tlsConf)
	case ProtoQUIC:
		d = quic.NewTunnelClient(cc.Address, tlsConf)
	case ProtoHTTP2:
		d = http2.NewTunnelClient(cc.Address, tlsConf, cc.Username, cc.Password)
	default:
		return nil, errors.New("unsupported proxy protocol: " + cc.Proto)
	}
	slog.Info(fmt.Sprintf("use proxy: %s %s", cc.Proto, cc.Address))
	return &proxyDialer{dialer: d, allowPrivate: cc.allowPrivate}, nil
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

// DialContext uses routes to determine direct or tunneled connection to host:port,
// returning a stream for subsequent read/write.
func (d *proxyDialer) DialContext(ctx context.Context, address string) (io.ReadWriteCloser, error) {
	// In production, requests to private host do not go through the proxy server
	if !d.allowPrivate {
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		if private(host) {
			var d net.Dialer
			return d.DialContext(ctx, "tcp", address)
		}
	}
	return d.dialer.DialContext(ctx, address)
}

func (d *proxyDialer) Close() error {
	c, ok := d.dialer.(io.Closer)
	if !ok {
		return nil
	}
	return c.Close()
}

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strings"

	"github.com/chenen3/yeager/cert"
	"github.com/chenen3/yeager/proxy"
	"github.com/chenen3/yeager/route"
	"github.com/chenen3/yeager/tunnel"
	"github.com/chenen3/yeager/tunnel/grpc"
	"github.com/chenen3/yeager/tunnel/http2"
	"github.com/chenen3/yeager/tunnel/quic"
)

// StartServices starts services with the given config,
// any started service will be return as io.Closer for future stopping
func StartServices(conf Config) ([]io.Closer, error) {
	if len(conf.Proxy) == 0 && len(conf.Listen) == 0 {
		return nil, errors.New("no proxy client nor server specified in config")
	}

	var services []io.Closer
	if len(conf.Proxy) > 0 {
		routes := conf.Routes
		if len(routes) == 0 {
			// routing through the first proxy client by default
			routes = []string{"final," + conf.Proxy[0].Name}
		}
		r, err := newRouter(routes, conf.Proxy)
		if err != nil {
			return nil, errors.New("init router: " + err.Error())
		}
		services = append(services, r)

		if conf.ListenHTTP != "" {
			lis, err := net.Listen("tcp", conf.ListenHTTP)
			if err != nil {
				return nil, err
			}
			hs := new(proxy.HTTPServer)
			go func() {
				slog.Info("listen http " + conf.ListenHTTP)
				if err := hs.Serve(lis, r.DialContext); err != nil {
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
				if err := ss.Serve(lis, r.DialContext); err != nil {
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

// router integrates tunnel dialers with routes
type router struct {
	dialers map[string]tunnel.Dialer
	routes  route.Routes
}

func newRouter(rules []string, clientConfigs []ClientConfig) (*router, error) {
	if len(rules) == 0 {
		return nil, errors.New("rules required")
	}

	var r router
	rs, err := route.New(rules)
	if err != nil {
		return nil, err
	}
	r.routes = rs

	dialers := make(map[string]tunnel.Dialer)
	for _, cc := range clientConfigs {
		if cc.Name == "" {
			return nil, fmt.Errorf("empty tunnel name")
		}
		name := strings.ToLower(cc.Name)
		if _, ok := dialers[name]; ok {
			return nil, fmt.Errorf("duplicated tunnel name: %s", name)
		}

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

		switch cc.Proto {
		case ProtoGRPC:
			client := grpc.NewTunnelClient(cc.Address, tlsConf)
			dialers[name] = client
		case ProtoQUIC:
			client := quic.NewTunnelClient(cc.Address, tlsConf)
			dialers[name] = client
		case ProtoHTTP2:
			client := http2.NewTunnelClient(cc.Address, tlsConf, cc.Username, cc.Password)
			dialers[name] = client
		default:
			slog.Warn("ignore unsupported tunnel", "route", cc.Name, "proto", cc.Proto)
			continue
		}
		slog.Info(fmt.Sprintf("register route %s: %s %s", cc.Name, cc.Proto, cc.Address))
	}
	r.dialers = dialers
	return &r, nil
}

// DialContext uses routes to determine direct or tunneled connection to host:port,
// returning a stream for subsequent read/write.
func (r *router) DialContext(ctx context.Context, target string) (io.ReadWriteCloser, error) {
	host, _, err := net.SplitHostPort(target)
	if err != nil {
		return nil, err
	}
	pass, err := r.routes.Match(host)
	if err != nil {
		return nil, err
	}

	switch pass {
	case route.Direct:
		slog.Debug("connect " + target)
		var d net.Dialer
		return d.DialContext(ctx, "tcp", target)
	case route.Reject:
		return nil, errors.New("route rejected")
	default:
		d, ok := r.dialers[pass]
		if !ok {
			return nil, errors.New("unknown route: " + pass)
		}
		slog.Debug(fmt.Sprintf("route %s to %s", target, pass))
		return d.DialContext(ctx, target)
	}
}

// Close closes all the tunnel dialers and return the first error encountered
func (r *router) Close() error {
	var err error
	for _, d := range r.dialers {
		if dc, ok := d.(io.Closer); ok {
			if e := dc.Close(); e != nil && err == nil {
				err = e
			}
		}
	}
	return err
}

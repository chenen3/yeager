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
	"github.com/chenen3/yeager/router"
	"github.com/chenen3/yeager/tunnel"
	"github.com/chenen3/yeager/tunnel/grpc"
	"github.com/chenen3/yeager/tunnel/http2"
	"github.com/chenen3/yeager/tunnel/quic"
)

// StartServices starts services with the given config,
// any started service will be return as io.Closer for future stopping
func StartServices(conf Config) ([]io.Closer, error) {
	var closers []io.Closer
	var rd *RouteDialer
	if conf.ListenHTTP != "" {
		lis, err := net.Listen("tcp", conf.ListenHTTP)
		if err != nil {
			return nil, err
		}
		if rd == nil {
			d, err := NewRouteDialer(conf.Rules, conf.Proxy)
			if err != nil {
				return nil, fmt.Errorf("init connector: %s", err)
			}
			rd = d
			closers = append(closers, rd)
		}
		hs := &httpProxy{}
		go func() {
			slog.Info("listen http " + conf.ListenHTTP)
			if err := hs.Serve(lis, rd.DialContext); err != nil {
				slog.Error("failed to serve http proxy: " + err.Error())
			}
		}()
		closers = append(closers, hs)
	}

	if conf.ListenSOCKS != "" {
		lis, err := net.Listen("tcp", conf.ListenSOCKS)
		if err != nil {
			return nil, err
		}
		if rd == nil {
			c, err := NewRouteDialer(conf.Rules, conf.Proxy)
			if err != nil {
				return nil, fmt.Errorf("init connector: %s", err)
			}
			rd = c
			closers = append(closers, rd)
		}
		ss := new(socksServer)
		go func() {
			slog.Info("listen socks " + conf.ListenSOCKS)
			if err := ss.Serve(lis, rd.DialContext); err != nil {
				slog.Error("failed to serve socks proxy: " + err.Error())
			}
		}()
		closers = append(closers, ss)
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
			closers = append(closers, &s)
		case ProtoQUIC:
			var s quic.TunnelServer
			go func() {
				if err := s.Serve(sc.Address, tlsConf); err != nil {
					slog.Error("start tunnel: "+err.Error(), "proto", sc.Proto)
				}
			}()
			closers = append(closers, &s)
		case ProtoHTTP2:
			var s http2.TunnelServer
			go func() {
				if err := s.Serve(sc.Address, tlsConf, sc.Username, sc.Password); err != nil {
					slog.Error("start tunnel: "+err.Error(), "proto", sc.Proto)
				}
			}()
			closers = append(closers, &s)
		}
		slog.Info(fmt.Sprintf("listen %s %s", sc.Proto, sc.Address))
	}
	return closers, nil
}

func CloseAll(closers []io.Closer) {
	for _, c := range closers {
		if err := c.Close(); err != nil {
			slog.Error("failed to close: " + err.Error())
		}
	}
}

// RouteDialer integrates tunnel dialers with router
type RouteDialer struct {
	dialers map[string]tunnel.Dialer
	router  router.Router
}

func NewRouteDialer(rules []string, clientConfigs []ClientConfig) (*RouteDialer, error) {
	var d RouteDialer
	if len(rules) > 0 {
		r, err := router.New(rules)
		if err != nil {
			return nil, err
		}
		d.router = r
	}

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
	d.dialers = dialers
	return &d, nil
}

// DialContext uses router to determine direct or tunneled connection to host:port,
// returning a stream for subsequent read/write.
func (d *RouteDialer) DialContext(ctx context.Context, target string) (io.ReadWriteCloser, error) {
	route := router.DefaultRoute
	if d.router != nil {
		host, _, err := net.SplitHostPort(target)
		if err != nil {
			return nil, err
		}
		r, e := d.router.Match(host)
		if e != nil {
			return nil, e
		}
		route = r
	}
	switch route {
	case router.RejectRoute:
		return nil, errors.New("route rejected")
	case router.DirectRoute:
		slog.Debug("connect " + target)
		var d net.Dialer
		return d.DialContext(ctx, "tcp", target)
	default:
		d, ok := d.dialers[route]
		if !ok {
			return nil, errors.New("unknown route: " + route)
		}
		slog.Debug(fmt.Sprintf("route %s to %s", target, route))
		return d.DialContext(ctx, target)
	}
}

// Close closes all the tunnel dialers and return the first error encountered
func (d *RouteDialer) Close() error {
	var err error
	for _, d := range d.dialers {
		if dc, ok := d.(io.Closer); ok {
			if e := dc.Close(); e != nil && err == nil {
				err = e
			}
		}
	}
	return err
}

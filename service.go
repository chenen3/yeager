package main

import (
	"context"
	"errors"
	"expvar"
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

var connStats = expvar.NewMap("connstats")

// StartServices starts services with the given config,
// any started service will be return as io.Closer for future stopping
func StartServices(conf Config) ([]io.Closer, error) {
	var closers []io.Closer
	var connector *Connector
	if conf.ListenHTTP != "" {
		lis, err := net.Listen("tcp", conf.ListenHTTP)
		if err != nil {
			return nil, err
		}
		if connector == nil {
			c, err := NewConnector(conf.Rules, conf.Proxy)
			if err != nil {
				return nil, fmt.Errorf("init connector: %s", err)
			}
			connector = c
			closers = append(closers, connector)
		}
		hs := newHTTPProxy()
		go func() {
			slog.Info("listen http " + conf.ListenHTTP)
			if err := hs.Serve(lis, connector.Connect); err != nil {
				slog.Error("failed to serve http proxy: " + err.Error())
			}
		}()
		closers = append(closers, hs)
		connStats.Set("http", expvar.Func(func() any {
			return hs.ConnNum()
		}))
	}

	if conf.ListenSOCKS != "" {
		lis, err := net.Listen("tcp", conf.ListenSOCKS)
		if err != nil {
			return nil, err
		}
		if connector == nil {
			c, err := NewConnector(conf.Rules, conf.Proxy)
			if err != nil {
				return nil, fmt.Errorf("init connector: %s", err)
			}
			connector = c
			closers = append(closers, connector)
		}
		ss := newSOCKServer()
		go func() {
			slog.Info("listen socks " + conf.ListenSOCKS)
			if err := ss.Serve(lis, connector.Connect); err != nil {
				slog.Error("failed to serve socks proxy: " + err.Error())
			}
		}()
		closers = append(closers, ss)
		connStats.Set("socks", expvar.Func(func() any {
			return ss.ConnNum()
		}))
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

// Connector integrates tunnel dialers with router
type Connector struct {
	dialers map[string]tunnel.Dialer
	router  router.Router
}

// NewConnector creates a new Connector for client side proxy
func NewConnector(rules []string, clientConfigs []ClientConfig) (*Connector, error) {
	var c Connector
	if len(rules) > 0 {
		r, err := router.New(rules)
		if err != nil {
			return nil, err
		}
		c.router = r
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
			connStats.Set(cc.Name, expvar.Func(func() any {
				return client.ConnNum()
			}))
		case ProtoQUIC:
			client := quic.NewTunnelClient(cc.Address, tlsConf)
			dialers[name] = client
			connStats.Set(cc.Name, expvar.Func(func() any {
				return client.ConnNum()
			}))
		case ProtoHTTP2:
			client := http2.NewTunnelClient(cc.Address, tlsConf, cc.Username, cc.Password)
			dialers[name] = client
			connStats.Set(cc.Name, expvar.Func(func() any {
				return client.ConnNum()
			}))
		default:
			slog.Warn("ignore unsupported tunnel", "route", cc.Name, "proto", cc.Proto)
			continue
		}
		slog.Info(fmt.Sprintf("register route %s: %s %s", cc.Name, cc.Proto, cc.Address))
	}
	c.dialers = dialers
	return &c, nil
}

// Connect uses router to determine direct or tunneled connection to host:port,
// returning a stream for subsequent read/write.
func (c *Connector) Connect(ctx context.Context, target string) (io.ReadWriteCloser, error) {
	route := router.DefaultRoute
	if c.router != nil {
		host, _, err := net.SplitHostPort(target)
		if err != nil {
			return nil, err
		}
		r, e := c.router.Match(host)
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
		d, ok := c.dialers[route]
		if !ok {
			return nil, errors.New("unknown route: " + route)
		}
		slog.Debug(fmt.Sprintf("route %s to %s", target, route))
		return d.DialContext(ctx, target)
	}
}

// Close closes all the tunnel dialers and return the first error encountered
func (c *Connector) Close() error {
	var err error
	for _, d := range c.dialers {
		if dc, ok := d.(io.Closer); ok {
			if e := dc.Close(); e != nil && err == nil {
				err = e
			}
		}
	}
	return err
}

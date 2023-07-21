package main

import (
	"context"
	"errors"
	"expvar"
	"fmt"
	"io"
	"log"
	"net"
	"strings"

	"github.com/chenen3/yeager/cert"
	"github.com/chenen3/yeager/config"
	"github.com/chenen3/yeager/debug"
	"github.com/chenen3/yeager/rule"
	"github.com/chenen3/yeager/tunnel"
	"github.com/chenen3/yeager/tunnel/grpc"
	"github.com/chenen3/yeager/tunnel/http2"
	"github.com/chenen3/yeager/tunnel/quic"
)

var connStats = expvar.NewMap("connstats")

// StartServices starts services with the given config,
// any started service will be return as io.Closer for future stopping
func StartServices(conf config.Config) ([]io.Closer, error) {
	var closers []io.Closer
	var tunneler *Tunneler
	if len(conf.Proxy) > 0 {
		t, err := NewTunneler(conf.Rules, conf.Proxy)
		if err != nil {
			return nil, fmt.Errorf("new tunneler: %s", err)
		}
		tunneler = t
		closers = append(closers, tunneler)
	}

	if conf.ListenHTTP != "" {
		lis, err := net.Listen("tcp", conf.ListenHTTP)
		if err != nil {
			return nil, err
		}
		if tunneler == nil {
			return nil, fmt.Errorf("tunnel client required")
		}
		hs := newHTTPProxy()
		go func() {
			log.Printf("http proxy listening %s", conf.ListenHTTP)
			if err := hs.Serve(lis, tunneler); err != nil {
				log.Printf("failed to serve http proxy: %s", err)
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
		if tunneler == nil {
			return nil, fmt.Errorf("tunnel client required")
		}
		ss := newSOCKServer()
		go func() {
			log.Printf("socks proxy listening %s", conf.ListenSOCKS)
			if err := ss.Serve(lis, tunneler); err != nil {
				log.Printf("failed to serve socks proxy: %s", err)
			}
		}()
		closers = append(closers, ss)
		connStats.Set("socks", expvar.Func(func() any {
			return ss.ConnNum()
		}))
	}

	for _, tl := range conf.Listen {
		tl := tl
		certPEM, err := tl.GetCertPEM()
		if err != nil {
			return nil, fmt.Errorf("read certificate: %s", err)
		}
		keyPEM, err := tl.GetKeyPEM()
		if err != nil {
			return nil, fmt.Errorf("read key: %s", err)
		}
		caPEM, err := tl.GetCAPEM()
		if err != nil {
			return nil, fmt.Errorf("read CA: %s", err)
		}
		tlsConf, err := cert.MakeServerTLSConfig(caPEM, certPEM, keyPEM)
		if err != nil {
			return nil, err
		}

		switch tl.Proto {
		case config.ProtoGRPC:
			lis, err := net.Listen("tcp", tl.Address)
			if err != nil {
				return nil, err
			}
			var s grpc.TunnelServer
			go func() {
				if err := s.Serve(lis, tlsConf); err != nil {
					log.Printf("%s tunnel serve: %s", tl.Proto, err)
				}
			}()
			closers = append(closers, &s)
		case config.ProtoQUIC:
			var s quic.TunnelServer
			go func() {
				if err := s.Serve(tl.Address, tlsConf); err != nil {
					log.Printf("%s tunnel serve: %s", tl.Proto, err)
				}
			}()
			closers = append(closers, &s)
		case config.ProtoHTTP2:
			var s http2.TunnelServer
			go func() {
				if err := s.Serve(tl.Address, tlsConf); err != nil {
					log.Printf("%s tunnel serve: %s", tl.Proto, err)
				}
			}()
			closers = append(closers, &s)
		}
		log.Printf("%s tunnel listening %s", tl.Proto, tl.Address)
	}
	return closers, nil
}

func CloseAll(closers []io.Closer) {
	for _, c := range closers {
		if err := c.Close(); err != nil {
			log.Printf("failed to close: %s", err)
		}
	}
}

// Tunneler integrates tunnel dialers with rules
type Tunneler struct {
	dialers map[string]tunnel.Dialer
	rules   rule.Rules
}

// NewTunneler creates a new Tunneler for client side proxy
func NewTunneler(rules []string, tunClients []config.TunnelClient) (*Tunneler, error) {
	var t Tunneler
	if len(rules) > 0 {
		r, err := rule.Parse(rules)
		if err != nil {
			return nil, err
		}
		t.rules = r
	}

	dialers := make(map[string]tunnel.Dialer)
	for _, tc := range tunClients {
		if tc.Name == "" {
			return nil, fmt.Errorf("empty tunnel name")
		}
		name := strings.ToLower(tc.Name)
		if _, ok := dialers[name]; ok {
			return nil, fmt.Errorf("duplicated tunnel name: %s", name)
		}

		certPEM, err := tc.GetCertPEM()
		if err != nil {
			return nil, fmt.Errorf("read certificate: %s", err)
		}
		keyPEM, err := tc.GetKeyPEM()
		if err != nil {
			return nil, fmt.Errorf("read key: %s", err)
		}
		caPEM, err := tc.GetCAPEM()
		if err != nil {
			return nil, fmt.Errorf("read CA: %s", err)
		}
		tlsConf, err := cert.MakeClientTLSConfig(caPEM, certPEM, keyPEM)
		if err != nil {
			log.Printf("certPEM: %s", certPEM)
			return nil, fmt.Errorf("make tls conf: %s", err)
		}

		switch tc.Proto {
		case config.ProtoGRPC:
			client := grpc.NewTunnelClient(tc.Address, tlsConf)
			dialers[name] = client
			connStats.Set(tc.Name, expvar.Func(func() any {
				return client.ConnNum()
			}))
		case config.ProtoQUIC:
			client := quic.NewTunnelClient(tc.Address, tlsConf)
			dialers[name] = client
			connStats.Set(tc.Name, expvar.Func(func() any {
				return client.ConnNum()
			}))
		case config.ProtoHTTP2:
			client := http2.NewTunnelClient(tc.Address, tlsConf)
			dialers[name] = client
			connStats.Set(tc.Name, expvar.Func(func() any {
				return client.ConnNum()
			}))
		default:
			log.Printf("ignore unsupported %s tunnel: %s", tc.Proto, tc.Name)
			continue
		}
		log.Printf("%s targeting %s tunnel %s", tc.Name, tc.Proto, tc.Address)
	}
	t.dialers = dialers
	return &t, nil
}

// DialContext connects to host:port target directly or through a tunnel, determined by the routing
func (t *Tunneler) DialContext(ctx context.Context, target string) (rwc io.ReadWriteCloser, err error) {
	policy := rule.Direct
	if t.rules != nil {
		host, _, err := net.SplitHostPort(target)
		if err != nil {
			return nil, err
		}
		p, e := t.rules.Match(host)
		if e != nil {
			return nil, e
		}
		policy = p
	}

	switch policy {
	case rule.Reject:
		return nil, errors.New("rejected by rules")
	case rule.Direct:
		debug.Printf("connect %s", target)
		var d net.Dialer
		return d.DialContext(ctx, "tcp", target)
	default:
		d, ok := t.dialers[policy]
		if !ok {
			return nil, fmt.Errorf("unknown proxy policy: %s", policy)
		}
		debug.Printf("connect %s via %s", target, policy)
		return d.DialContext(ctx, target)
	}
}

// Close closes all the tunnel dialers and return the first error encountered
func (t *Tunneler) Close() error {
	var err error
	for _, d := range t.dialers {
		if c, ok := d.(io.Closer); ok {
			if e := c.Close(); e != nil && err == nil {
				err = e
			}
		}
	}
	return err
}

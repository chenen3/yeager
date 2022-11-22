package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"strings"

	"github.com/chenen3/yeager/config"
	"github.com/chenen3/yeager/httpproxy"
	"github.com/chenen3/yeager/route"
	"github.com/chenen3/yeager/socks"
	"github.com/chenen3/yeager/tunnel"
	"github.com/chenen3/yeager/tunnel/grpc"
	"github.com/chenen3/yeager/tunnel/quic"
)

func CloseAll(closers []io.Closer) {
	for _, c := range closers {
		if err := c.Close(); err != nil {
			log.Printf("failed to close: %s", err)
		}
	}
}

// StartServices starts services with the given config,
// any started service will be return as io.Closer for future stopping
func StartServices(conf config.Config) ([]io.Closer, error) {
	var closers []io.Closer
	var tunneler *Tunneler
	if len(conf.TunnelClients) > 0 {
		t, err := NewTunneler(conf.Rules, conf.TunnelClients, conf.Verbose)
		if err != nil {
			return nil, fmt.Errorf("failed to init dispatcher: %s", err)
		}
		tunneler = t
		closers = append(closers, tunneler)
	}

	if conf.HTTPListen != "" {
		if tunneler == nil {
			return nil, fmt.Errorf("tunnel client required")
		}
		var hs httpproxy.Server
		closers = append(closers, &hs)
		go func() {
			log.Printf("http proxy listening %s", conf.HTTPListen)
			if err := hs.Serve(conf.HTTPListen, tunneler); err != nil {
				log.Printf("failed to serve http proxy: %s", err)
			}
		}()
	}

	if conf.SOCKSListen != "" {
		if tunneler == nil {
			return nil, fmt.Errorf("tunnel client required")
		}
		var ss socks.Server
		closers = append(closers, &ss)
		go func() {
			log.Printf("socks proxy listening %s", conf.SOCKSListen)
			if err := ss.Serve(conf.SOCKSListen, tunneler); err != nil {
				log.Printf("failed to serve socks proxy: %s", err)
			}
		}()
	}

	for _, tl := range conf.TunnelListens {
		tl := tl
		switch tl.Type {
		case config.TunGRPC:
			tlsConf, err := makeServerTLSConfig(tl)
			if err != nil {
				return nil, err
			}
			var s grpc.TunnelServer
			closers = append(closers, &s)
			go func() {
				log.Printf("%s tunnel listening %s", tl.Type, tl.Listen)
				if err := s.Serve(tl.Listen, tlsConf); err != nil {
					log.Printf("%s tunnel serve: %s", tl.Type, err)
				}
			}()
		case config.TunQUIC:
			tlsConf, err := makeServerTLSConfig(tl)
			if err != nil {
				return nil, err
			}
			var s quic.TunnelServer
			closers = append(closers, &s)
			go func() {
				log.Printf("%s tunnel listening %s", tl.Type, tl.Listen)
				if err := s.Serve(tl.Listen, tlsConf); err != nil {
					log.Printf("%s tunnel serve: %s", tl.Type, err)
				}
			}()
		}
	}
	return closers, nil
}

// Tunneler integrates tunnel dialers with router
type Tunneler struct {
	dialers map[string]tunnel.Dialer
	router  *route.Router
	verbose bool
	closers []io.Closer
}

func NewTunneler(rules []string, tunClients []config.TunnelClient, verbose bool) (*Tunneler, error) {
	var t Tunneler
	if len(rules) > 0 {
		r, err := route.NewRouter(rules)
		if err != nil {
			return nil, err
		}
		t.router = r
	}

	dialers := make(map[string]tunnel.Dialer)
	for _, tc := range tunClients {
		policy := strings.ToLower(tc.Policy)
		if _, ok := dialers[policy]; ok {
			return nil, fmt.Errorf("duplicated tunnel policy: %s", policy)
		}
		switch tc.Type {
		case config.TunGRPC:
			tlsConf, err := makeClientTLSConfig(tc)
			if err != nil {
				return nil, err
			}
			client := grpc.NewTunnelClient(tc.Address, tlsConf, tc.ConnectionPoolSize)
			dialers[policy] = client
			// clean up connection pool
			t.closers = append(t.closers, client)
			log.Printf("%s targeting GRPC tunnel %s", tc.Policy, tc.Address)
		case config.TunQUIC:
			tlsConf, err := makeClientTLSConfig(tc)
			if err != nil {
				return nil, err
			}
			client := quic.NewTunnelClient(tc.Address, tlsConf, tc.ConnectionPoolSize)
			dialers[policy] = client
			// clean up connection pool
			t.closers = append(t.closers, client)
			log.Printf("%s targeting QUIC tunnel %s", tc.Policy, tc.Address)
		default:
			return nil, fmt.Errorf("unknown tunnel type: %s", tc.Type)
		}
	}
	t.dialers = dialers
	t.verbose = verbose
	return &t, nil
}

// DialContext connects to address directly or through a tunnel, determined by the routing
func (t *Tunneler) DialContext(ctx context.Context, address string) (rwc io.ReadWriteCloser, err error) {
	policy := route.Direct
	if t.router != nil {
		p, e := t.router.Dispatch(address)
		if e != nil {
			return nil, e
		}
		policy = p
	}

	switch policy {
	case route.Reject:
		return nil, errors.New("rule rejected")
	case route.Direct:
		if t.verbose {
			log.Printf("%s -> %s", address, policy)
		}
		var d net.Dialer
		return d.DialContext(ctx, "tcp", address)
	default:
		d, ok := t.dialers[policy]
		if !ok {
			return nil, fmt.Errorf("unknown proxy policy: %s", policy)
		}
		if t.verbose {
			log.Printf("%s -> %s", address, policy)
		}
		return d.DialContext(ctx, address)
	}
}

// Close closes all the tunnel dialers and return the first error encountered
func (t *Tunneler) Close() error {
	var err error
	for _, c := range t.closers {
		e := c.Close()
		if e != nil && err == nil {
			err = e
		}
	}
	return err
}

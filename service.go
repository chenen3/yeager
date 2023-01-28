package main

import (
	"context"
	"errors"
	"expvar"
	"fmt"
	"io"
	"log"
	"net"
	"strconv"
	"strings"

	"github.com/chenen3/yeager/cert"
	"github.com/chenen3/yeager/config"
	"github.com/chenen3/yeager/debug"
	"github.com/chenen3/yeager/httpproxy"
	"github.com/chenen3/yeager/rule"
	"github.com/chenen3/yeager/socks"
	"github.com/chenen3/yeager/tunnel"
	"github.com/chenen3/yeager/tunnel/grpc"
	"github.com/chenen3/yeager/tunnel/quic"
)

func init() {
	expvar.Publish("conn", connCount)
}

var connCount = new(counters)

type counters []func() int

func (c *counters) RegistCounter(f func() int) {
	*c = append(*c, f)
}

func (c *counters) String() string {
	var total int
	for _, f := range *c {
		total += f()
	}
	return strconv.FormatInt(int64(total), 10)
}

// StartServices starts services with the given config,
// any started service will be return as io.Closer for future stopping
func StartServices(conf config.Config) ([]io.Closer, error) {
	var closers []io.Closer
	var tunneler *Tunneler
	if len(conf.TunnelClients) > 0 {
		t, err := NewTunneler(conf.Rules, conf.TunnelClients)
		if err != nil {
			return nil, fmt.Errorf("new tunneler: %s", err)
		}
		tunneler = t
		closers = append(closers, tunneler)
	}

	if conf.HTTPListen != "" {
		lis, err := net.Listen("tcp", conf.HTTPListen)
		if err != nil {
			return nil, err
		}
		if tunneler == nil {
			return nil, fmt.Errorf("tunnel client required")
		}
		var hs httpproxy.Server
		go func() {
			log.Printf("http proxy listening %s", conf.HTTPListen)
			if err := hs.Serve(lis, tunneler); err != nil {
				log.Printf("failed to serve http proxy: %s", err)
			}
		}()
		closers = append(closers, &hs)
		connCount.RegistCounter(hs.Len)
	}

	if conf.SOCKSListen != "" {
		lis, err := net.Listen("tcp", conf.SOCKSListen)
		if err != nil {
			return nil, err
		}
		if tunneler == nil {
			return nil, fmt.Errorf("tunnel client required")
		}
		var ss socks.Server
		go func() {
			log.Printf("socks proxy listening %s", conf.SOCKSListen)
			if err := ss.Serve(lis, tunneler); err != nil {
				log.Printf("failed to serve socks proxy: %s", err)
			}
		}()
		closers = append(closers, &ss)
		connCount.RegistCounter(ss.Len)
	}

	for _, tl := range conf.TunnelListens {
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

		switch tl.Type {
		case config.TunGRPC:
			lis, err := net.Listen("tcp", tl.Listen)
			if err != nil {
				return nil, err
			}
			var s grpc.TunnelServer
			go func() {
				log.Printf("%s tunnel listening %s", tl.Type, tl.Listen)
				if err := s.Serve(lis, tlsConf); err != nil {
					log.Printf("%s tunnel serve: %s", tl.Type, err)
				}
			}()
			closers = append(closers, &s)
		case config.TunQUIC:
			var s quic.TunnelServer
			go func() {
				log.Printf("%s tunnel listening %s", tl.Type, tl.Listen)
				if err := s.Serve(tl.Listen, tlsConf); err != nil {
					log.Printf("%s tunnel serve: %s", tl.Type, err)
				}
			}()
			closers = append(closers, &s)
		}
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
	closers []io.Closer
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
		policy := strings.ToLower(tc.Policy)
		if _, ok := dialers[policy]; ok {
			return nil, fmt.Errorf("duplicated tunnel policy: %s", policy)
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

		switch tc.Type {
		case config.TunGRPC:
			client := grpc.NewTunnelClient(tc.Address, tlsConf)
			dialers[policy] = client
			// clean up connections
			t.closers = append(t.closers, client)
			log.Printf("%s targeting GRPC tunnel %s", tc.Policy, tc.Address)
		case config.TunQUIC:
			client := quic.NewTunnelClient(tc.Address, tlsConf)
			dialers[policy] = client
			// clean up connections
			t.closers = append(t.closers, client)
			log.Printf("%s targeting QUIC tunnel %s", tc.Policy, tc.Address)
		default:
			return nil, fmt.Errorf("unknown tunnel type: %s", tc.Type)
		}
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
		return nil, errors.New("rule rejected")
	case rule.Direct:
		if debug.Enabled() {
			log.Printf("connect %s", target)
		}
		var d net.Dialer
		return d.DialContext(ctx, "tcp", target)
	default:
		d, ok := t.dialers[policy]
		if !ok {
			return nil, fmt.Errorf("unknown proxy policy: %s", policy)
		}
		if debug.Enabled() {
			log.Printf("connect %s via %s", target, policy)
		}
		return d.DialContext(ctx, target)
	}
}

// Close closes all the tunnel dialers and return the first error encountered
func (t *Tunneler) Close() error {
	var err error
	for _, c := range t.closers {
		if e := c.Close(); e != nil && err == nil {
			err = e
		}
	}
	return err
}

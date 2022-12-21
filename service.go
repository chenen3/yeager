package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strings"

	"github.com/chenen3/yeager/cert"
	"github.com/chenen3/yeager/config"
	"github.com/chenen3/yeager/httpproxy"
	ylog "github.com/chenen3/yeager/log"
	"github.com/chenen3/yeager/route"
	"github.com/chenen3/yeager/socks"
	"github.com/chenen3/yeager/tunnel"
	"github.com/chenen3/yeager/tunnel/grpc"
	"github.com/chenen3/yeager/tunnel/quic"
)

// StartServices starts services with the given config,
// any started service will be return as io.Closer for future stopping
func StartServices(conf config.Config) ([]io.Closer, error) {
	var closers []io.Closer
	var tunneler *Tunneler
	if len(conf.TunnelClients) > 0 {
		t, err := NewTunneler(conf.Rules, conf.TunnelClients)
		if err != nil {
			return nil, fmt.Errorf("failed to init dispatcher: %s", err)
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
		closers = append(closers, &hs)
		go func() {
			log.Printf("http proxy listening %s", conf.HTTPListen)
			if err := hs.Serve(lis, tunneler); err != nil {
				log.Printf("failed to serve http proxy: %s", err)
			}
		}()
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
		closers = append(closers, &ss)
		go func() {
			log.Printf("socks proxy listening %s", conf.SOCKSListen)
			if err := ss.Serve(lis, tunneler); err != nil {
				log.Printf("failed to serve socks proxy: %s", err)
			}
		}()
	}

	for _, tl := range conf.TunnelListens {
		tl := tl
		certPEM, err := readDataOrFile(tl.CertPEM, tl.CertFile)
		if err != nil {
			return nil, fmt.Errorf("read TLS certificate: %s", err)
		}
		keyPEM, err := readDataOrFile(tl.KeyPEM, tl.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("read TLS key: %s", err)
		}
		caPEM, err := readDataOrFile(tl.CAPEM, tl.CAFile)
		if err != nil {
			return nil, fmt.Errorf("read TLS CA: %s", err)
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

// Tunneler integrates tunnel dialers with router
type Tunneler struct {
	dialers map[string]tunnel.Dialer
	router  *route.Router
	closers []io.Closer
}

// NewTunneler creates a new Tunneler for client side proxy
func NewTunneler(rules []string, tunClients []config.TunnelClient) (*Tunneler, error) {
	var t Tunneler
	if len(rules) > 0 {
		r, err := route.New(rules)
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

		certPEM, err := readDataOrFile(tc.CertPEM, tc.CertFile)
		if err != nil {
			return nil, fmt.Errorf("read TLS certificate: %s", err)
		}
		keyPEM, err := readDataOrFile(tc.KeyPEM, tc.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("read TLS key: %s", err)
		}
		caPEM, err := readDataOrFile(tc.CAPEM, tc.CAFile)
		if err != nil {
			return nil, fmt.Errorf("read TLS CA: %s", err)
		}
		tlsConf, err := cert.MakeClientTLSConfig(caPEM, certPEM, keyPEM)
		if err != nil {
			return nil, err
		}

		switch tc.Type {
		case config.TunGRPC:
			client := grpc.NewTunnelClient(tc.Address, tlsConf, tc.ConnectionPoolSize)
			dialers[policy] = client
			// clean up connection pool
			t.closers = append(t.closers, client)
			log.Printf("%s targeting GRPC tunnel %s", tc.Policy, tc.Address)
		case config.TunQUIC:
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
	return &t, nil
}

// DialContext connects to host:port target directly or through a tunnel, determined by the routing
func (t *Tunneler) DialContext(ctx context.Context, target string) (rwc io.ReadWriteCloser, err error) {
	policy := route.Direct
	if t.router != nil {
		host, _, err := net.SplitHostPort(target)
		if err != nil {
			return nil, err
		}
		p, e := t.router.Dispatch(host)
		if e != nil {
			return nil, e
		}
		policy = p
	}

	switch policy {
	case route.Reject:
		return nil, errors.New("rule rejected")
	case route.Direct:
		ylog.Debugf("connect %s", target)
		var d net.Dialer
		return d.DialContext(ctx, "tcp", target)
	default:
		d, ok := t.dialers[policy]
		if !ok {
			return nil, fmt.Errorf("unknown proxy policy: %s", policy)
		}
		ylog.Debugf("connect %s via %s", target, policy)
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

func readDataOrFile(data string, filename string) ([]byte, error) {
	if data != "" {
		return []byte(data), nil
	}
	if filename != "" {
		return os.ReadFile(filename)
	}
	return nil, errors.New("no data nor filename provided")
}

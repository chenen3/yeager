package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"runtime"
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
		err := c.Close()
		if err != nil {
			log.Printf("failed to close: %s", err)
		}
	}
}

// StartServices starts services with the given config,
// any started service will be return as io.Closer for future stopping
func StartServices(conf config.Config) ([]io.Closer, error) {
	var closers []io.Closer
	var dispatcher *Dispatcher
	if len(conf.TunnelClients) > 0 {
		d, err := NewDispatcher(conf.Rules, conf.TunnelClients, conf.Verbose)
		if err != nil {
			return nil, fmt.Errorf("failed to init dispatcher: %s", err)
		}
		dispatcher = d
		closers = append(closers, dispatcher)
		// TODO
		// reduce the memory usage boosted by parsing rules of geosite.dat
		runtime.GC()
	}

	if conf.HTTPListen != "" {
		if dispatcher == nil {
			return nil, fmt.Errorf("tunnel client required")
		}
		var hs httpproxy.Server
		closers = append(closers, &hs)
		go func() {
			log.Printf("http proxy listening %s", conf.HTTPListen)
			err := hs.Serve(conf.HTTPListen, dispatcher)
			if err != nil {
				log.Printf("failed to serve http proxy: %s", err)
			}
		}()
	}

	if conf.SOCKSListen != "" {
		if dispatcher == nil {
			return nil, fmt.Errorf("tunnel client required")
		}
		var ss socks.Server
		closers = append(closers, &ss)
		go func() {
			log.Printf("socks proxy listening %s", conf.SOCKSListen)
			err := ss.Serve(conf.SOCKSListen, dispatcher)
			if err != nil {
				log.Printf("failed to serve socks proxy: %s", err)
			}
		}()
	}

	for _, tl := range conf.TunnelListens {
		tl := tl
		switch tl.Type {
		case config.TunTCP:
			var t tunnel.TcpTunnelServer
			closers = append(closers, &t)
			go func() {
				log.Printf("%s tunnel listening %s", tl.Type, tl.Listen)
				err := t.Serve(tl.Listen)
				if err != nil {
					log.Printf("%s tunnel serve: %s", tl.Type, err)
				}
			}()
		case config.TunGRPC:
			tlsConf, err := makeServerTLSConfig(tl)
			if err != nil {
				return nil, err
			}
			var s grpc.TunnelServer
			closers = append(closers, &s)
			go func() {
				log.Printf("%s tunnel listening %s", tl.Type, tl.Listen)
				err := s.Serve(tl.Listen, tlsConf)
				if err != nil {
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
				err := s.Serve(tl.Listen, tlsConf)
				if err != nil {
					log.Printf("%s tunnel serve: %s", tl.Type, err)
				}
			}()
		}
	}
	return closers, nil
}

type Tunneler interface {
	DialContext(ctx context.Context, addr string) (io.ReadWriteCloser, error)
}

// Dispatcher dispatches traffic to tunnels, determined by router rules
type Dispatcher struct {
	tunnels map[string]Tunneler
	router  *route.Router
	verbose bool
	closers []io.Closer
}

func NewDispatcher(rules []string, tunnelClients []config.TunnelClient, verbose bool) (*Dispatcher, error) {
	var d Dispatcher
	if len(rules) > 0 {
		r, err := route.NewRouter(rules)
		if err != nil {
			return nil, err
		}
		d.router = r
	}

	tunnels := make(map[string]Tunneler)
	for _, tc := range tunnelClients {
		policy := strings.ToLower(tc.Policy)
		if _, ok := tunnels[policy]; ok {
			return nil, fmt.Errorf("duplicated tunnel policy: %s", policy)
		}
		switch tc.Type {
		case config.TunTCP:
			tunnels[policy] = tunnel.NewTcpTunnelClient(tc.Address)
		case config.TunGRPC:
			tlsConf, err := makeClientTLSConfig(tc)
			if err != nil {
				return nil, err
			}
			client := grpc.NewTunnelClient(tc.Address, tlsConf, tc.ConnectionPoolSize)
			tunnels[policy] = client
			// clean up connection pool
			d.closers = append(d.closers, client)
		case config.TunQUIC:
			tlsConf, err := makeClientTLSConfig(tc)
			if err != nil {
				return nil, err
			}
			client := quic.NewTunnelClient(tc.Address, tlsConf, tc.ConnectionPoolSize)
			tunnels[policy] = client
			// clean up connection pool
			d.closers = append(d.closers, client)
		default:
			return nil, fmt.Errorf("unknown tunnel type: %s", tc.Type)
		}
	}
	d.tunnels = tunnels
	d.verbose = verbose
	return &d, nil
}

// DialContext connects to the given address via tunnel or directly
func (d *Dispatcher) DialContext(ctx context.Context, addr string) (rwc io.ReadWriteCloser, err error) {
	policy := route.Direct
	if d.router != nil {
		p, e := d.router.Dispatch(addr)
		if e != nil {
			return nil, e
		}
		policy = p
	}

	if policy == route.Reject {
		return nil, errors.New("rule rejected")
	}

	if policy == route.Direct {
		if d.verbose {
			log.Printf("%s -> %s", addr, policy)
		}
		var dialer net.Dialer
		return dialer.DialContext(ctx, "tcp", addr)
	}

	tunnel, ok := d.tunnels[policy]
	if !ok {
		return nil, fmt.Errorf("unknown proxy policy: %s", policy)
	}
	if d.verbose {
		log.Printf("%s -> %s", addr, policy)
	}
	return tunnel.DialContext(ctx, addr)
}

func (d *Dispatcher) Close() error {
	var err error
	for _, c := range d.closers {
		e := c.Close()
		if e != nil {
			err = e
		}
	}
	return err
}

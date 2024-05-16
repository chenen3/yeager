package main

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/chenen3/yeager/config"
	"github.com/chenen3/yeager/logger"
	"github.com/chenen3/yeager/proxy"
	"github.com/chenen3/yeager/transport"
	"github.com/chenen3/yeager/transport/grpc"
	"github.com/chenen3/yeager/transport/http2"
	"github.com/chenen3/yeager/transport/shadowsocks"
)

// start the service specified by config.
// The caller should call stop when finished.
func start(cfg config.Config) (stop func(), err error) {
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

	if len(cfg.Transport) == 0 && len(cfg.Listen) == 0 {
		return nil, errors.New("missing client and server config")
	}

	var dialer transport.StreamDialer
	getDialer := func() (transport.StreamDialer, error) {
		if dialer != nil {
			return dialer, nil
		}
		if len(cfg.Transport) == 0 {
			return nil, errors.New("missing transport config")
		}
		d, err := newStreamDialers(cfg.Transport, cfg.NoProxy)
		if err != nil {
			return nil, err
		}
		if v, ok := d.(io.Closer); ok {
			onStop = append(onStop, v.Close)
		}
		dialer = d
		return dialer, nil
	}

	for _, c := range cfg.Listen {
		switch c.Protocol {
		case config.ProtoHTTP:
			dialer, err := getDialer()
			if err != nil {
				return nil, err
			}
			listener, err := net.Listen("tcp", c.Address)
			if err != nil {
				return nil, err
			}
			s := &http.Server{Handler: proxy.NewHTTPHandler(dialer)}
			go func() {
				err := s.Serve(listener)
				if err != nil && err != http.ErrServerClosed {
					logger.Error.Printf("serve http: %s", err)
				}
			}()
			onStop = append(onStop, s.Close)
		case config.ProtoSOCKS5:
			dialer, err := getDialer()
			if err != nil {
				return nil, err
			}
			listener, err := net.Listen("tcp", c.Address)
			if err != nil {
				return nil, err
			}
			s := proxy.NewSOCKS5Server(dialer)
			go func() {
				err := s.Serve(listener)
				if err != nil {
					logger.Error.Printf("serve socks5: %s", err)
				}
			}()
			onStop = append(onStop, s.Close)
		case config.ProtoGRPC:
			tlsConf, err := c.ServerTLS()
			if err != nil {
				return nil, err
			}
			s, err := grpc.NewServer(c.Address, tlsConf)
			if err != nil {
				return nil, err
			}
			onStop = append(onStop, func() error {
				s.Stop()
				return nil
			})
		case config.ProtoHTTP2:
			tlsConf, err := c.ServerTLS()
			if err != nil {
				return nil, err
			}
			s, err := http2.NewServer(c.Address, tlsConf, c.Username, c.Password)
			if err != nil {
				return nil, err
			}
			onStop = append(onStop, s.Close)
		default:
			return nil, errors.New("unknown protocol: " + c.Protocol)
		}
		logger.Info.Printf("listen %s %s", c.Protocol, c.Address)
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

func newStreamDialer(c config.ServerConfig) (transport.StreamDialer, error) {
	var dialer transport.StreamDialer
	switch c.Protocol {
	case config.ProtoGRPC:
		tlsConf, err := c.ClientTLS()
		if err != nil {
			return nil, err
		}
		dialer = grpc.NewStreamDialer(c.Address, tlsConf)
	case config.ProtoHTTP2:
		if c.Username != "" {
			dialer = http2.NewStreamDialer(c.Address, nil, c.Username, c.Password)
		} else {
			tlsConf, err := c.ClientTLS()
			if err != nil {
				return nil, err
			}
			dialer = http2.NewStreamDialer(c.Address, tlsConf, "", "")
		}
	case config.ProtoShadowsocks:
		d, err := shadowsocks.NewStreamDialer(c.Address, c.Cipher, c.Secret)
		if err != nil {
			return nil, err
		}
		dialer = d
	default:
		return nil, errors.New("unsupported transport protocol: " + c.Protocol)
	}
	return dialer, nil
}

type dialerGroup struct {
	transports []config.ServerConfig
	ticker     *time.Ticker
	mu         sync.RWMutex
	transport  config.ServerConfig
	dialer     transport.StreamDialer
	noProxy    *noProxyConfig
}

// newStreamDialers returns a new stream dialer.
// Given multiple transport config, it creates a dialer group to
// perform periodic health checks and switch server if necessary.
func newStreamDialers(transports []config.ServerConfig, noProxy string) (transport.StreamDialer, error) {
	if len(transports) == 0 {
		return nil, errors.New("missing transport config")
	}

	g := &dialerGroup{noProxy: parseNoProxy(noProxy)}
	if len(transports) == 1 {
		d, err := newStreamDialer(transports[0])
		if err != nil {
			return nil, err
		}
		g.dialer = d
		return g, nil
	}

	g.transports = transports
	g.ticker = time.NewTicker(15 * time.Second)
	go g.healthCheck()
	return g, nil
}

func (g *dialerGroup) healthCheck() {
	for range g.ticker.C {
		conn, err := net.DialTimeout("tcp", g.transport.Address, 5*time.Second)
		if err != nil {
			logger.Debug.Printf("health check: %s", err)
			g.pick()
			continue
		}
		conn.Close()
	}
}

// pick a healthy proxy server and set up the dialer, prioritizing low latency
func (g *dialerGroup) pick() {
	g.mu.Lock()
	defer g.mu.Unlock()

	// If the current proxy server is working, don't change.
	conn, err := net.DialTimeout("tcp", g.transport.Address, 5*time.Second)
	if err == nil {
		conn.Close()
		return
	}

	var transport config.ServerConfig
	var min int
	for _, t := range g.transports {
		start := time.Now()
		c, e := net.DialTimeout("tcp", t.Address, 5*time.Second)
		if e != nil {
			logger.Debug.Printf("tcp ping: %s", e)
			continue
		}
		defer c.Close()
		latency := time.Since(start)
		if min == 0 || int(latency) < min {
			transport = t
			min = int(latency)
		}
		logger.Debug.Printf("tcp ping %s, latency %dms", t.Address, latency.Milliseconds())
	}
	if transport.Address == "" {
		logger.Error.Println("no healthy proxy server")
		return
	}

	d, err := newStreamDialer(transport)
	if err != nil {
		logger.Error.Printf("new stream dialer: %s", err)
		return
	}
	if v, ok := g.dialer.(io.Closer); ok {
		v.Close()
	}
	g.dialer = d
	g.transport = transport
	logger.Info.Printf("pick transport: %s %s", transport.Protocol, transport.Address)
}

// implements interface transport.StreamDialer
func (g *dialerGroup) Dial(ctx context.Context, address string) (transport.Stream, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	if g.noProxy.contains(address) {
		var d net.Dialer
		conn, err := d.DialContext(ctx, "tcp", address)
		if err != nil {
			return nil, err
		}
		logger.Debug.Printf("connected to %s, no proxy", address)
		return conn.(*net.TCPConn), nil
	}
	if g.dialer == nil {
		g.mu.RUnlock()
		g.pick()
		g.mu.RLock()
	}
	stream, err := g.dialer.Dial(ctx, address)
	if err != nil {
		return nil, err
	}
	logger.Debug.Printf("connected to %s", address)
	return stream, nil
}

func (g *dialerGroup) Close() error {
	g.mu.RLock()
	defer g.mu.RUnlock()
	if g.ticker != nil {
		g.ticker.Stop()
	}
	if v, ok := g.dialer.(io.Closer); ok {
		return v.Close()
	}
	return nil
}

type noProxyConfig struct {
	ipMatchers     []matcher
	domainMatchers []matcher
}

func parseNoProxy(noProxy string) *noProxyConfig {
	var cfg noProxyConfig
	for _, p := range strings.Split(noProxy, ",") {
		p = strings.ToLower(strings.TrimSpace(p))
		if len(p) == 0 {
			continue
		}

		if p == "*" {
			cfg.ipMatchers = []matcher{allMatch{}}
			cfg.domainMatchers = []matcher{allMatch{}}
			break
		}

		// IP/CIDR
		if _, pnet, err := net.ParseCIDR(p); err == nil {
			cfg.ipMatchers = append(cfg.ipMatchers, cidrMatch{cidr: pnet})
			continue
		}

		// IP
		if pip := net.ParseIP(p); pip != nil {
			cfg.ipMatchers = append(cfg.ipMatchers, ipMatch{ip: pip})
			continue
		}

		// domain name
		phost := strings.TrimPrefix(p, "*.")
		cfg.domainMatchers = append(cfg.domainMatchers, domainMatch{host: phost})
	}
	return &cfg
}

func (c *noProxyConfig) contains(addr string) bool {
	if len(addr) == 0 || len(c.ipMatchers)+len(c.domainMatchers) == 0 {
		return false
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)

	if ip != nil {
		for _, m := range c.ipMatchers {
			if m.match("", ip) {
				return true
			}
		}
		return false
	}

	for _, m := range c.domainMatchers {
		if m.match(host, nil) {
			return true
		}
	}
	return false
}

// matcher represents the matching rule for a given value in the NO_PROXY list
type matcher interface {
	// match returns true if the host or ip are allowed
	match(host string, ip net.IP) bool
}

// allMatch matches on all possible inputs
type allMatch struct{}

func (a allMatch) match(host string, ip net.IP) bool {
	return true
}

type cidrMatch struct {
	cidr *net.IPNet
}

func (m cidrMatch) match(host string, ip net.IP) bool {
	return m.cidr.Contains(ip)
}

type ipMatch struct {
	ip net.IP
}

func (m ipMatch) match(host string, ip net.IP) bool {
	return m.ip.Equal(ip)
}

// domainMatch matches a domain name and all subdomains.
// For example "foo.com" matches "foo.com" and "bar.foo.com", but not "xfoo.com"
type domainMatch struct {
	host string
}

func (m domainMatch) match(host string, ip net.IP) bool {
	before, found := strings.CutSuffix(host, m.host)
	if !found {
		return false
	}
	return before == "" || before[len(before)-1] == '.'
}

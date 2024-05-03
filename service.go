package main

import (
	"context"
	"errors"
	"io"
	"math"
	"net"
	"net/http"
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

	var trans []config.Transport
	if cfg.Transport.Address != "" {
		trans = append(trans, cfg.Transport)
	}
	for _, t := range cfg.Transports {
		if t.Address != "" {
			trans = append(trans, t)
		}
	}

	if len(trans) == 0 && len(cfg.Listen) == 0 {
		return nil, errors.New("missing client or server config")
	}

	if len(trans) > 0 {
		dialer, err := newStreamDialers(trans)
		if err != nil {
			return nil, err
		}
		if v, ok := dialer.(io.Closer); ok {
			onStop = append(onStop, v.Close)
		}

		if cfg.HTTPProxy != "" {
			listener, err := net.Listen("tcp", cfg.HTTPProxy)
			if err != nil {
				return nil, err
			}
			s := &http.Server{Handler: proxy.NewHTTPHandler(dialer)}
			go func() {
				logger.Info.Printf("listen HTTP %s", cfg.HTTPProxy)
				err := s.Serve(listener)
				if err != nil && err != http.ErrServerClosed {
					logger.Error.Printf("serve http: %s", err)
				}
			}()
			onStop = append(onStop, s.Close)
		}

		if cfg.SOCKSProxy != "" {
			listener, err := net.Listen("tcp", cfg.SOCKSProxy)
			if err != nil {
				return nil, err
			}
			ss := proxy.NewSOCKS5Server(dialer)
			go func() {
				logger.Info.Printf("listen SOCKS5 %s", cfg.SOCKSProxy)
				err := ss.Serve(listener)
				if err != nil {
					logger.Error.Printf("serve socks: %s", err)
				}
			}()
			onStop = append(onStop, ss.Close)
		}
	}

	for _, t := range cfg.Listen {
		tlsConf, err := t.ServerTLS()
		if err != nil {
			return nil, err
		}
		switch t.Protocol {
		case config.ProtoGRPC:
			s, err := grpc.NewServer(t.Address, tlsConf)
			if err != nil {
				return nil, err
			}
			onStop = append(onStop, func() error {
				s.Stop()
				return nil
			})
		case config.ProtoHTTP2:
			s, err := http2.NewServer(t.Address, tlsConf, t.Username, t.Password)
			if err != nil {
				return nil, err
			}
			onStop = append(onStop, s.Close)
		default:
			return nil, errors.New("unknown protocol: " + t.Protocol)
		}
		logger.Info.Printf("listen %s %s", t.Protocol, t.Address)
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

func newStreamDialer(c config.Transport) (transport.StreamDialer, error) {
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
	configs []config.Transport

	ticker    *time.Ticker
	mu        sync.RWMutex
	transport config.Transport
	dialer    transport.StreamDialer
}

// newStreamDialers create a stream dialer for transport config.
// If there are multiple transports, it creates a dialer group to
// perform periodic health checks and switch dialer if necessary.
func newStreamDialers(configs []config.Transport) (transport.StreamDialer, error) {
	if len(configs) == 0 {
		return nil, errors.New("missing transport config")
	}
	if len(configs) == 1 {
		return newStreamDialer(configs[0])
	}

	g := &dialerGroup{
		configs: configs,
		ticker:  time.NewTicker(15 * time.Second),
	}
	go g.healthCheck()
	return g, nil
}

// pick an available transport, prioritizing low latency
func pickTransport(ts []config.Transport) (config.Transport, error) {
	var picked config.Transport
	timed := math.MaxInt
	// following connection test may take a while.
	for _, t := range ts {
		start := time.Now()
		c, e := net.DialTimeout("tcp", t.Address, 5*time.Second)
		if e != nil {
			logger.Debug.Printf("health check: %s", e)
			continue
		}
		defer c.Close()
		du := time.Since(start)
		if int(du) < timed {
			picked = t
			timed = int(du)
		}
		logger.Debug.Printf("health check %s, latency %dms", t.Address, du.Milliseconds())
	}
	if picked.Address == "" {
		return config.Transport{}, errors.New("no healthy transport")
	}
	return picked, nil
}

func (g *dialerGroup) healthCheck() {
	for range g.ticker.C {
		conn, err := net.DialTimeout("tcp", g.transport.Address, 5*time.Second)
		if err != nil {
			g.pick()
			continue
		}
		conn.Close()
	}
}

func (g *dialerGroup) pick() {
	g.mu.Lock()
	defer g.mu.Unlock()

	// if current transport is fine, return early
	conn, err := net.DialTimeout("tcp", g.transport.Address, 5*time.Second)
	if err == nil {
		conn.Close()
		return
	}

	tr, err := pickTransport(g.configs)
	if err != nil {
		logger.Error.Println(err)
		return
	}
	d, err := newStreamDialer(tr)
	if err != nil {
		logger.Error.Printf("new stream dialer: %s", err)
		return
	}
	g.transport = tr
	if v, ok := g.dialer.(io.Closer); ok {
		v.Close()
	}
	g.dialer = d
	logger.Info.Printf("pick transport %s %s", tr.Protocol, tr.Address)
}

// implements interface transport.StreamDialer
func (g *dialerGroup) Dial(ctx context.Context, address string) (transport.Stream, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()
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
	g.ticker.Stop()
	if v, ok := g.dialer.(io.Closer); ok {
		return v.Close()
	}
	return nil
}

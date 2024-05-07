package main

import (
	"context"
	"errors"
	"io"
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
		d, err := newStreamDialers(cfg.Transport)
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
}

// newStreamDialers returns a new stream dialer.
// Given multiple transport config, it creates a dialer group to
// perform periodic health checks and switch server if necessary.
func newStreamDialers(transports []config.ServerConfig) (transport.StreamDialer, error) {
	if len(transports) == 0 {
		return nil, errors.New("missing transport config")
	}
	if len(transports) == 1 {
		return newStreamDialer(transports[0])
	}

	g := &dialerGroup{
		transports: transports,
		ticker:     time.NewTicker(15 * time.Second),
	}
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

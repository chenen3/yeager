package main

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
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

	if len(cfg.Listen) == 0 && cfg.Transport.Address == "" && len(cfg.Transports) == 0 {
		return nil, errors.New("missing client or server config")
	}

	if cfg.Transport.Address != "" || len(cfg.Transports) > 0 {
		dialer, err := newDialerWrapper(cfg.Transport, cfg.Transports)
		if err != nil {
			return nil, err
		}
		onStop = append(onStop, dialer.Close)

		if cfg.HTTPProxy != "" {
			listener, err := net.Listen("tcp", cfg.HTTPProxy)
			if err != nil {
				return nil, err
			}
			s := &http.Server{Handler: proxy.NewHTTPHandler(dialer)}
			go func() {
				logger.Info.Printf("listen HTTP proxy: %s", cfg.HTTPProxy)
				err := s.Serve(listener)
				if err != nil && err != http.ErrServerClosed {
					logger.Error.Printf("serve http proxy: %s", err)
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
				logger.Info.Printf("listen SOCKS5 proxy: %s", cfg.SOCKSProxy)
				err := ss.Serve(listener)
				if err != nil {
					logger.Error.Printf("serve socks proxy: %s", err)
				}
			}()
			onStop = append(onStop, ss.Close)
		}
	}

	for _, t := range cfg.Listen {
		certPEM, err := t.Cert()
		if err != nil {
			return nil, fmt.Errorf("read certificate: %s", err)
		}
		keyPEM, err := t.Key()
		if err != nil {
			return nil, fmt.Errorf("read key: %s", err)
		}
		caPEM, err := t.CA()
		if err != nil {
			return nil, fmt.Errorf("read CA: %s", err)
		}
		tlsConf, err := config.NewServerTLS(caPEM, certPEM, keyPEM)
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
	var tlsConf *tls.Config
	if c.Protocol != config.ProtoHTTP2 || c.Username == "" || c.Password == "" {
		certPEM, err := c.Cert()
		if err != nil {
			return nil, fmt.Errorf("read certificate: %s", err)
		}
		keyPEM, err := c.Key()
		if err != nil {
			return nil, fmt.Errorf("read key: %s", err)
		}
		caPEM, err := c.CA()
		if err != nil {
			return nil, fmt.Errorf("read CA: %s", err)
		}
		tlsConf, err = config.NewClientTLS(caPEM, certPEM, keyPEM)
		if err != nil {
			return nil, fmt.Errorf("make tls conf: %s", err)
		}
	}

	var d transport.StreamDialer
	switch c.Protocol {
	case config.ProtoGRPC:
		d = grpc.NewStreamDialer(c.Address, tlsConf)
	case config.ProtoHTTP2:
		d = http2.NewStreamDialer(c.Address, tlsConf, c.Username, c.Password)
	default:
		return nil, errors.New("unsupported transport protocol: " + c.Protocol)
	}
	return d, nil
}

type dialerWrapper struct {
	trans      config.Transport
	candidates []config.Transport
	ticker     *time.Ticker
	mu         sync.RWMutex
	dialer     transport.StreamDialer
}

// create a stream dialer with fallback.
// If the current transport is not available,
// the wrapper will try to use another avaiable transport.
func newDialerWrapper(tr config.Transport, candidates []config.Transport) (*dialerWrapper, error) {
	w := &dialerWrapper{
		trans:      tr,
		candidates: append(candidates, tr),
		ticker:     time.NewTicker(30 * time.Second),
	}
	if w.trans.Address == "" {
		go w.fallback()
		return w, nil
	}

	d, err := newStreamDialer(tr)
	if err != nil {
		return nil, err
	}
	w.dialer = d
	return w, nil
}

// pick an available transport, prioritizing low latency
func pick(ts []config.Transport) (config.Transport, error) {
	var picked config.Transport
	timed := math.MaxInt
	// following connection test may take a while.
	for _, t := range ts {
		if t.Address == "" {
			continue
		}
		start := time.Now()
		c, e := net.DialTimeout("tcp", t.Address, 5*time.Second)
		if e != nil {
			logger.Debug.Printf("dial transport: %s", e)
			continue
		}
		defer c.Close()
		du := time.Since(start)
		if int(du) < timed {
			picked = t
			timed = int(du)
		}
		logger.Debug.Printf("dial transport %s cost %dms", t.Address, du.Milliseconds())
	}
	if picked.Address == "" {
		return config.Transport{}, errors.New("no available transport")
	}
	return picked, nil
}

func (b *dialerWrapper) tryFallback() {
	select {
	case <-b.ticker.C:
		go b.fallback()
	default:
	}
}

func (w *dialerWrapper) fallback() {
	w.mu.Lock()
	defer w.mu.Unlock()

	// if current transport is fine, return early
	conn, err := net.DialTimeout("tcp", w.trans.Address, 5*time.Second)
	if err == nil {
		conn.Close()
		return
	}

	tr, err := pick(w.candidates)
	if err != nil {
		logger.Error.Println(err)
		return
	}
	d, err := newStreamDialer(tr)
	if err != nil {
		logger.Error.Printf("new stream dialer: %s", err)
		return
	}
	w.trans = tr
	if v, ok := w.dialer.(io.Closer); ok {
		v.Close()
	}
	w.dialer = d
	logger.Info.Printf("fallback to transport %s %s", tr.Protocol, tr.Address)
}

// implements interface transport.StreamDialer
func (w *dialerWrapper) Dial(ctx context.Context, address string) (transport.Stream, error) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if w.dialer == nil {
		w.tryFallback()
		return nil, errors.New("no available transport")
	}
	stream, err := w.dialer.Dial(ctx, address)
	if err != nil {
		w.tryFallback()
		return nil, err
	}
	return stream, nil
}

func (w *dialerWrapper) Close() error {
	w.mu.RLock()
	defer w.mu.RUnlock()
	w.ticker.Stop()
	if v, ok := w.dialer.(io.Closer); ok {
		return v.Close()
	}
	return nil
}

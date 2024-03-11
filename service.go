package main

import (
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"

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

	if len(cfg.Listen) == 0 && cfg.Transport.Address == "" {
		return nil, errors.New("missing client or server config")
	}

	if cfg.Transport.Address != "" {
		dialer, err := newStreamDialer(cfg.Transport)
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
		}
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

package main

import (
	"context"
	"errors"
	"expvar"
	"io"
	"log"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/chenen3/yeager/config"
	"github.com/chenen3/yeager/http"
	"github.com/chenen3/yeager/route"
	"github.com/chenen3/yeager/socks"
	"github.com/chenen3/yeager/tunnel"
	"github.com/chenen3/yeager/util"
)

type Inbounder interface {
	// RegisterHandler register handler function for incoming connection.
	// Inbounder is responsible for closing the incoming connection,
	// not the handler function.
	RegisterHandler(func(ctx context.Context, conn net.Conn, addr string))
	// ListenAndServe start the proxy server,
	// block until closed or encounter error
	ListenAndServe() error
	Close() error
}

type Outbounder interface {
	DialContext(ctx context.Context, network string, addr string) (net.Conn, error)
}

// reject implements Outbounder,
// always reject connection and return error
type reject struct{}

func (reject) DialContext(_ context.Context, _, _ string) (net.Conn, error) {
	return nil, errors.New("rejected")
}

type Proxy struct {
	conf      config.Config
	router    *route.Router
	outbounds map[string]Outbounder
	inbounds  []Inbounder
}

func NewProxy(conf config.Config) (*Proxy, error) {
	p := &Proxy{
		conf:      conf,
		outbounds: make(map[string]Outbounder, 2+len(conf.Outbounds)),
	}

	if conf.SOCKSListen != "" {
		srv, err := socks.NewServer(conf.SOCKSListen)
		if err != nil {
			return nil, errors.New("init socks5 server: " + err.Error())
		}
		p.inbounds = append(p.inbounds, srv)
	}
	if conf.HTTPListen != "" {
		srv, err := http.NewProxyServer(conf.HTTPListen)
		if err != nil {
			return nil, errors.New("init http proxy server: " + err.Error())
		}
		p.inbounds = append(p.inbounds, srv)
	}
	for _, ib := range conf.Inbounds {
		ib := ib
		srv, err := tunnel.NewServer(&ib)
		if err != nil {
			return nil, errors.New("init yeager proxy server: " + err.Error())
		}
		p.inbounds = append(p.inbounds, srv)
	}

	// built-in outbound
	p.outbounds[route.Direct] = new(net.Dialer)
	p.outbounds[route.Reject] = reject{}

	for _, oc := range conf.Outbounds {
		outbound, err := tunnel.NewClient(&oc)
		if err != nil {
			return nil, err
		}
		tag := strings.ToLower(oc.Tag)
		if _, ok := p.outbounds[tag]; ok {
			return nil, errors.New("duplicated outbound tag: " + oc.Tag)
		}
		p.outbounds[tag] = outbound
	}

	if len(conf.Rules) > 0 {
		rt, err := route.NewRouter(conf.Rules)
		if err != nil {
			return nil, err
		}
		p.router = rt
	}
	return p, nil
}

// Start starts proxy services
func (p *Proxy) Start() {
	var wg sync.WaitGroup
	for _, inbound := range p.inbounds {
		wg.Add(1)
		go func(ib Inbounder) {
			defer wg.Done()
			ib.RegisterHandler(p.handleConn)
			err := ib.ListenAndServe()
			if err != nil && !errors.Is(err, net.ErrClosed) {
				log.Printf("inbound server exited abnormally: %s", err)
			}
		}(inbound)
	}
	for _, oc := range p.conf.Outbounds {
		log.Printf("tunnel: %s, addr: %s, transport: %s \n", oc.Tag, oc.Address, oc.Transport)
	}
	wg.Wait()
}

// Stop stops proxy services
func (p *Proxy) Stop() error {
	var err error
	for _, ib := range p.inbounds {
		if e := ib.Close(); e != nil {
			err = e
		}
	}

	for _, outbound := range p.outbounds {
		if c, ok := outbound.(io.Closer); ok {
			if e := c.Close(); e != nil {
				err = e
			}
		}
	}

	return err
}

var numConn = expvar.NewInt("numConn")

func (p *Proxy) handleConn(ctx context.Context, inconn net.Conn, addr string) {
	if p.conf.Debug {
		numConn.Add(1)
		defer numConn.Add(-1)
	}

	// when no rule provided, dial directly
	tag := route.Direct
	if p.router != nil {
		t, err := p.router.Dispatch(addr)
		if err != nil {
			log.Printf("dispatch %s: %s", addr, err)
			return
		}
		tag = t
	}
	outbound, ok := p.outbounds[tag]
	if !ok {
		log.Printf("unknown outbound tag: %s", tag)
		return
	}

	if p.conf.Verbose {
		log.Printf("relay %s <-> %s <-> %s", inconn.RemoteAddr(), tag, addr)
	}

	dctx, cancel := context.WithTimeout(context.Background(), util.DialTimeout)
	defer cancel()
	outconn, err := outbound.DialContext(dctx, "tcp", addr)
	if err != nil {
		log.Printf("connect %s: %s", addr, err)
		return
	}
	defer outconn.Close()

	errCh := make(chan error, 1)
	r := relay{inConn: inconn, outConn: outconn}
	go r.copyToOutbound(errCh)
	go r.copyFromOutbound(errCh)

	select {
	case <-ctx.Done():
	case <-errCh:
	}
}

type relay struct {
	inConn  net.Conn
	outConn net.Conn
}

func (r *relay) copyToOutbound(errCh chan<- error) {
	_, err := io.Copy(r.outConn, r.inConn)
	r.outConn.SetReadDeadline(time.Now().Add(5 * time.Second))
	errCh <- err
}

func (r *relay) copyFromOutbound(errCh chan<- error) {
	_, err := io.Copy(r.inConn, r.outConn)
	r.inConn.SetReadDeadline(time.Now().Add(5 * time.Second))
	errCh <- err
}

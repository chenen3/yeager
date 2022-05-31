package main

import (
	"context"
	"errors"
	"expvar"
	"io"
	"log"
	"net"
	"os"
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
	// Handle register handler function for incoming connection.
	// Inbounder is responsible for closing the incoming connection,
	// not the handler function.
	Handle(func(ctx context.Context, conn net.Conn, addr string))
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
	return nil, errors.New("traffic rejected")
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

	if conf.Inbounds.SOCKS != nil {
		srv, err := socks.NewServer(conf.Inbounds.SOCKS.Listen)
		if err != nil {
			return nil, errors.New("init socks5 server: " + err.Error())
		}
		p.inbounds = append(p.inbounds, srv)
	}
	if conf.Inbounds.HTTP != nil {
		srv, err := http.NewProxyServer(conf.Inbounds.HTTP.Listen)
		if err != nil {
			return nil, errors.New("init http proxy server: " + err.Error())
		}
		p.inbounds = append(p.inbounds, srv)
	}
	if conf.Inbounds.Yeager != nil {
		srv, err := tunnel.NewServer(conf.Inbounds.Yeager)
		if err != nil {
			return nil, errors.New("init yeager proxy server: " + err.Error())
		}
		p.inbounds = append(p.inbounds, srv)
	}
	if len(p.inbounds) == 0 {
		return nil, errors.New("no inbound specified in config")
	}

	// built-in outbound
	p.outbounds[route.Direct] = new(net.Dialer)
	p.outbounds[route.Reject] = reject{}

	for _, oc := range conf.Outbounds {
		outbound, err := tunnel.NewClient(oc)
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

// Serve launch inbound server and register handler for incoming connection.
// Serve returns when all inbounds stop.
func (p *Proxy) Serve() {
	var wg sync.WaitGroup
	for _, inbound := range p.inbounds {
		wg.Add(1)
		go func(ib Inbounder) {
			defer wg.Done()
			ib.Handle(p.handle)
			if err := ib.ListenAndServe(); err != nil {
				log.Printf("inbound server exit: %s", err)
				return
			}
		}(inbound)
	}
	wg.Wait()
}

func (p *Proxy) Close() error {
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

func (p *Proxy) handle(ctx context.Context, ic net.Conn, addr string) {
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
		log.Printf("relay %s <-> %s <-> %s", ic.RemoteAddr(), tag, addr)
	}

	dctx, cancel := context.WithTimeout(context.Background(), util.DialTimeout)
	defer cancel()
	oc, err := outbound.DialContext(dctx, "tcp", addr)
	if err != nil {
		log.Printf("connect %s: %s", addr, err)
		return
	}
	defer oc.Close()

	errCh := make(chan error, 1)
	go func() {
		inboundErrCh := make(chan error, 1)
		go func() {
			_, err := io.Copy(oc, ic)
			inboundErrCh <- err
			// unblock Read on outbound connection
			oc.SetReadDeadline(time.Now().Add(5 * time.Second))
			// grpc stream does nothing on SetReadDeadline()
			if cs, ok := oc.(interface{ CloseSend() error }); ok {
				cs.CloseSend()
			}
		}()

		_, errOb := io.Copy(ic, oc)
		// unblock Read on inbound connection
		ic.SetReadDeadline(time.Now().Add(5 * time.Second))

		errIb := <-inboundErrCh
		if errIb != nil && !errors.Is(errIb, os.ErrDeadlineExceeded) {
			errCh <- errors.New("relay from inbound: " + errIb.Error())
			return
		}
		if errOb != nil && !errors.Is(errOb, os.ErrDeadlineExceeded) {
			errCh <- errors.New("relay from outbound: " + errOb.Error())
			return
		}
		close(errCh)
	}()

	select {
	case <-ctx.Done():
	case err := <-errCh:
		// avoid confusing user by insignificant logs
		if err != nil && p.conf.Verbose {
			log.Print(err)
		}
	}
}

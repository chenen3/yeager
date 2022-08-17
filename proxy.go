package main

import (
	"context"
	"errors"
	"expvar"
	"fmt"
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

// Start starts proxy service
func (p *Proxy) Start() {
	var wg sync.WaitGroup
	for _, inbound := range p.inbounds {
		wg.Add(1)
		go func(ib Inbounder) {
			defer wg.Done()
			ib.Handle(p.handle)
			err := ib.ListenAndServe()
			if err != nil && !errors.Is(err, net.ErrClosed) {
				log.Printf("inbound server exited abnormally: %s", err)
			}
		}(inbound)
	}
	wg.Wait()
}

// Stop stops proxy service
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

func GenerateConfig(host string) (srv, cli config.Config, err error) {
	cert, err := util.GenerateCertificate(host)
	if err != nil {
		return srv, cli, err
	}
	tunnelPort, err := util.ChoosePort()
	if err != nil {
		return srv, cli, err
	}

	srv = config.Config{
		Inbounds: []config.YeagerServer{
			{
				Listen:    fmt.Sprintf(":%d", tunnelPort),
				Transport: config.TransGRPC,
				TLS: config.TLS{
					CAPEM:   string(cert.RootCert),
					CertPEM: string(cert.ServerCert),
					KeyPEM:  string(cert.ServerKey),
				},
			},
		},
		Rules: []string{"final,direct"},
	}

	socksProxyPort, err := util.ChoosePort()
	if err != nil {
		return srv, cli, err
	}
	httpProxyPort, err := util.ChoosePort()
	if err != nil {
		return srv, cli, err
	}
	cli = config.Config{
		SOCKSListen: fmt.Sprintf("127.0.0.1:%d", socksProxyPort),
		HTTPListen:  fmt.Sprintf("127.0.0.1:%d", httpProxyPort),
		Outbounds: []config.YeagerClient{
			{
				Tag:       "proxy",
				Address:   fmt.Sprintf("%s:%d", host, tunnelPort),
				Transport: config.TransGRPC,
				TLS: config.TLS{
					CAPEM:   string(cert.RootCert),
					CertPEM: string(cert.ClientCert),
					KeyPEM:  string(cert.ClientKey),
				},
			},
		},
		Rules: []string{"final,proxy"},
	}
	return srv, cli, nil
}

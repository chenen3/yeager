package proxy

import (
	"context"
	"errors"
	"expvar"
	"io"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/chenen3/yeager/config"
	"github.com/chenen3/yeager/log"
	"github.com/chenen3/yeager/proxy/common"
	"github.com/chenen3/yeager/proxy/http"
	"github.com/chenen3/yeager/proxy/socks"
	"github.com/chenen3/yeager/proxy/yeager"
	"github.com/chenen3/yeager/route"
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
		srv, err := http.NewServer(conf.Inbounds.HTTP.Listen)
		if err != nil {
			return nil, errors.New("init http proxy server: " + err.Error())
		}
		p.inbounds = append(p.inbounds, srv)
	}
	if conf.Inbounds.Yeager != nil {
		srv, err := yeager.NewServer(conf.Inbounds.Yeager)
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
		outbound, err := yeager.NewClient(oc)
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
				log.Errorf("inbound server exit: %s", err)
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

func (p *Proxy) handle(ctx context.Context, ibConn net.Conn, addr string) {
	if p.conf.Debug {
		numConn.Add(1)
		defer numConn.Add(-1)
	}

	// when no rule provided, dial directly
	tag := route.Direct
	if p.router != nil {
		t, err := p.router.Dispatch(addr)
		if err != nil {
			log.Errorf("dispatch %s: %s", addr, err)
			return
		}
		tag = t
	}
	outbound, ok := p.outbounds[tag]
	if !ok {
		log.Errorf("unknown outbound tag: %s", tag)
		return
	}

	if p.conf.Debug {
		log.Infof("peer %s, dest %s, outbound %s", ibConn.RemoteAddr(), addr, tag)
	}

	dctx, cancel := context.WithTimeout(context.Background(), common.DialTimeout)
	defer cancel()
	obConn, err := outbound.DialContext(dctx, "tcp", addr)
	if err != nil {
		log.Errorf("failed to connect %s: %s", addr, err)
		return
	}
	defer obConn.Close()

	errCh := make(chan error)
	go func() {
		inboundErrCh := make(chan error, 1)
		go func() {
			_, ibErr := io.Copy(obConn, ibConn)
			inboundErrCh <- ibErr
			// unblock Read on outbound connection
			obConn.SetReadDeadline(time.Now().Add(5 * time.Second))
			// grpc stream does nothing on SetReadDeadline()
			if cs, ok := obConn.(interface{ CloseSend() error }); ok {
				cs.CloseSend()
			}
		}()

		_, obErr := io.Copy(ibConn, obConn)
		// unblock Read on inbound connection
		ibConn.SetReadDeadline(time.Now().Add(5 * time.Second))

		ibErr := <-inboundErrCh
		if ibErr != nil && !errors.Is(ibErr, os.ErrDeadlineExceeded) {
			errCh <- errors.New("failed to relay traffic from inbound: " + ibErr.Error())
			return
		}
		if obErr != nil && !errors.Is(obErr, os.ErrDeadlineExceeded) {
			errCh <- errors.New("failed to relay traffic from outbound: " + obErr.Error())
			return
		}
		close(errCh)
	}()

	select {
	case <-ctx.Done():
	case err := <-errCh:
		// avoid confusing the average user by insignificant logs
		if err != nil && p.conf.Debug {
			log.Errorf(err.Error())
		}
	}
}

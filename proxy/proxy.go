package proxy

import (
	"context"
	"errors"
	"expvar"
	"io"
	"net"
	"strings"
	"sync"

	"github.com/chenen3/yeager/config"
	"github.com/chenen3/yeager/log"
	"github.com/chenen3/yeager/proxy/common"
	"github.com/chenen3/yeager/proxy/direct"
	"github.com/chenen3/yeager/proxy/http"
	"github.com/chenen3/yeager/proxy/reject"
	"github.com/chenen3/yeager/proxy/socks"
	"github.com/chenen3/yeager/proxy/yeager"
	"github.com/chenen3/yeager/route"
)

type Inbounder interface {
	// Handle register handler function for incomming connection,
	// handler is responsible for closing connection when Read and Write done
	Handle(func(c net.Conn, addr string))
	// ListenAndServe start the proxy server,
	// block until closed or encounter error
	ListenAndServe() error
	Close() error
}

type Outbounder interface {
	DialContext(ctx context.Context, network string, addr string) (net.Conn, error)
}

type Proxy struct {
	conf      *config.Config
	router    *route.Router
	outbounds map[string]Outbounder
	inbounds  []Inbounder
}

func NewProxy(conf *config.Config) (*Proxy, error) {
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
	p.outbounds[direct.Direct.String()] = direct.Direct
	p.outbounds[reject.Reject.String()] = reject.Reject

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

	rt, err := route.NewRouter(conf.Rules)
	if err != nil {
		return nil, err
	}
	p.router = rt
	return p, nil
}

// Serve launch inbound server and register handler for incoming connection.
// Serve returns when one of the inbounds stop.
func (p *Proxy) Serve() {
	var wg sync.WaitGroup
	for _, inbound := range p.inbounds {
		wg.Add(1)
		go func(ib Inbounder) {
			defer wg.Done()
			ib.Handle(p.handle)
			if err := ib.ListenAndServe(); err != nil {
				log.L().Error(err)
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

var activeConnCnt = expvar.NewInt("activeConn")

func (p *Proxy) handle(inConn net.Conn, addr string) {
	if p.conf.Debug {
		activeConnCnt.Add(1)
		defer activeConnCnt.Add(-1)
	}
	defer inConn.Close()

	tag, err := p.router.Dispatch(addr)
	if err != nil {
		log.L().Errorf("dispatch %s: %s", addr, err)
		return
	}
	outbound, ok := p.outbounds[tag]
	if !ok {
		log.L().Errorf("unknown outbound tag: %s", tag)
		return
	}
	log.L().Infof("receive %s from %s, dispatch to [%s]", addr, inConn.RemoteAddr(), tag)

	ctx, cancel := context.WithTimeout(context.Background(), common.DialTimeout)
	defer cancel()
	outConn, err := outbound.DialContext(ctx, "tcp", addr)
	if err != nil {
		log.L().Errorf("dial %s: %s", addr, err)
		return
	}
	defer outConn.Close()

	err = relay(inConn, outConn)
	if err != nil {
		log.L().Warnf("relay %s: %s", addr, err)
		return
	}
}

func relay(a, b io.ReadWriter) error {
	errCh := make(chan error, 2)
	go func() {
		_, err := io.Copy(a, b)
		errCh <- err
	}()
	go func() {
		_, err := io.Copy(b, a)
		errCh <- err
	}()
	return <-errCh
}

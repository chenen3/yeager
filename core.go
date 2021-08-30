package yeager

import (
	"context"
	"errors"
	"io"
	glog "log"
	"net"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"yeager/config"
	"yeager/log"
	"yeager/proxy"
	"yeager/proxy/armin"
	"yeager/proxy/direct"
	"yeager/proxy/http"
	"yeager/proxy/reject"
	"yeager/proxy/socks"
	"yeager/router"
)

var activeConn prometheus.Gauge

func init() {
	activeConn = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "active_connections",
		Help: "Number of active connections.",
	})
	prometheus.MustRegister(activeConn)
}

type Proxy struct {
	inbounds  []proxy.Inbound
	outbounds map[string]proxy.Outbound
	router    *router.Router
}

func NewProxy(conf *config.Config) (*Proxy, error) {
	p := &Proxy{
		outbounds: make(map[string]proxy.Outbound, 2+len(conf.Outbounds)),
	}

	if conf.Inbounds.SOCKS != nil {
		srv := socks.NewServer(conf.Inbounds.SOCKS)
		p.inbounds = append(p.inbounds, srv)
	}
	if conf.Inbounds.HTTP != nil {
		srv := http.NewServer(conf.Inbounds.HTTP)
		p.inbounds = append(p.inbounds, srv)
	}
	if conf.Inbounds.Armin != nil {
		srv := armin.NewServer(conf.Inbounds.Armin)
		p.inbounds = append(p.inbounds, srv)
	}

	// built-in outbound
	p.outbounds[direct.Tag] = new(direct.Client)
	p.outbounds[reject.Tag] = new(reject.Client)

	for _, oc := range conf.Outbounds {
		outbound, err := armin.NewClient(oc)
		if err != nil {
			return nil, err
		}
		tag := strings.ToLower(oc.Tag)
		if _, ok := p.outbounds[tag]; ok {
			return nil, errors.New("duplicated outbound tag: " + oc.Tag)
		}
		p.outbounds[tag] = outbound
	}

	rt, err := router.NewRouter(conf.Rules)
	if err != nil {
		return nil, err
	}
	p.router = rt
	return p, nil
}

func (p *Proxy) Start(ctx context.Context) {
	for _, inbound := range p.inbounds {
		inbound.RegisterHandler(p.handle)
		go func(inbound proxy.Inbound) {
			log.Error(inbound.ListenAndServe(ctx))
		}(inbound)
	}

	// cleanup if context ends
	defer func() {
		for _, outbound := range p.outbounds {
			if c, ok := outbound.(io.Closer); ok {
				c.Close()
			}
		}
	}()

	<-ctx.Done()
}

func (p *Proxy) handle(ctx context.Context, inConn net.Conn, addr *proxy.Address) {
	activeConn.Inc()
	defer activeConn.Dec()

	defer inConn.Close()
	tag := p.router.Dispatch(addr)
	outbound, ok := p.outbounds[tag]
	if !ok {
		log.Errorf("unknown outbound tag: %s", tag)
		return
	}
	glog.Printf("dispatch %s from %s to [%s]\n", addr, inConn.RemoteAddr(), tag)

	dialCtx, cancel := context.WithTimeout(ctx, proxy.DialTimeout)
	defer cancel()
	outConn, err := outbound.DialContext(dialCtx, addr)
	if err != nil {
		log.Error(err)
		return
	}
	defer outConn.Close()

	errCh := link(inConn, outConn)
	select {
	case <-ctx.Done():
	case err := <-errCh:
		if err != nil {
			log.Warnf("%s, dst %s", err, addr)
		}
	}
}

func link(a, b io.ReadWriter) <-chan error {
	errCh := make(chan error, 2)
	go func() {
		_, err := io.Copy(b, a)
		errCh <- err
	}()
	go func() {
		_, err := io.Copy(a, b)
		errCh <- err
	}()

	return errCh
}

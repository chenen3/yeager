package proxy

import (
	"context"
	"errors"
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
	"github.com/prometheus/client_golang/prometheus"
)

var activeConn prometheus.Gauge

func init() {
	activeConn = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "active_connections",
		Help: "Number of active connections.",
	})
	prometheus.MustRegister(activeConn)
}

type inbounder interface {
	// ListenAndServe start the proxy server and block until closed or encounter error
	ListenAndServe(handle func(ctx context.Context, conn net.Conn, addr string)) error
	Close() error
}

type outbounder interface {
	DialContext(ctx context.Context, addr string) (net.Conn, error)
}

type Proxy struct {
	inbounds  []inbounder
	outbounds map[string]outbounder
	router    *route.Router
	done      chan struct{}
}

func NewProxy(conf *config.Config) (*Proxy, error) {
	p := &Proxy{
		outbounds: make(map[string]outbounder, 2+len(conf.Outbounds)),
		done:      make(chan struct{}),
	}

	if conf.Inbounds.SOCKS != nil {
		srv := socks.NewServer(conf.Inbounds.SOCKS)
		p.inbounds = append(p.inbounds, srv)
	}
	if conf.Inbounds.HTTP != nil {
		srv := http.NewServer(conf.Inbounds.HTTP)
		p.inbounds = append(p.inbounds, srv)
	}
	if conf.Inbounds.Yeager != nil {
		srv := yeager.NewServer(conf.Inbounds.Yeager)
		p.inbounds = append(p.inbounds, srv)
	}
	if len(p.inbounds) == 0 {
		return nil, errors.New("no inbound specified in config")
	}

	// built-in outbound
	p.outbounds[direct.Tag] = direct.Direct
	p.outbounds[reject.Tag] = reject.Reject

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
func (p *Proxy) Serve() error {
	var wg sync.WaitGroup
	var once sync.Once
	for _, inbound := range p.inbounds {
		wg.Add(1)
		go func(ib inbounder) {
			defer wg.Done()
			err := ib.ListenAndServe(p.handle)
			if err != nil {
				log.Error(err)
				once.Do(p.Close)
				return
			}
		}(inbound)
	}

	wg.Wait()
	<-p.done
	return nil
}

func (p *Proxy) Close() {
	defer close(p.done)
	for _, ib := range p.inbounds {
		ib.Close()
	}

	for _, outbound := range p.outbounds {
		if c, ok := outbound.(io.Closer); ok {
			c.Close()
		}
	}
}

func (p *Proxy) handle(ctx context.Context, inConn net.Conn, addr string) {
	activeConn.Inc()
	defer activeConn.Dec()
	defer inConn.Close()

	tag, err := p.router.Dispatch(addr)
	if err != nil {
		log.Error(err)
		return
	}
	outbound, ok := p.outbounds[tag]
	if !ok {
		log.Errorf("unknown outbound tag: %s", tag)
		return
	}
	log.Infof("dispatch %s from %s to [%s]\n", addr, inConn.RemoteAddr(), tag)

	dialCtx, cancel := context.WithTimeout(ctx, common.DialTimeout)
	defer cancel()
	outConn, err := outbound.DialContext(dialCtx, addr)
	if err != nil {
		log.Error(err)
		return
	}
	defer outConn.Close()

	errCh := make(chan error, 2)
	go func() {
		_, err := io.Copy(outConn, inConn)
		errCh <- err
	}()
	go func() {
		_, err := io.Copy(inConn, outConn)
		errCh <- err
	}()

	select {
	case <-ctx.Done():
	case err := <-errCh:
		if err != nil {
			log.Warnf("%s, dst %s", err, addr)
		}
	}
}

package yeager

import (
	"context"
	"errors"
	"io"
	glog "log"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"yeager/config"
	"yeager/log"
	"yeager/proxy"
	_ "yeager/proxy/armin"
	"yeager/proxy/direct"
	_ "yeager/proxy/http"
	"yeager/proxy/reject"
	_ "yeager/proxy/socks"
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

func NewProxy(c config.Config) (*Proxy, error) {
	p := &Proxy{
		outbounds: make(map[string]proxy.Outbound, 2+len(c.Outbounds)),
	}
	for _, conf := range c.Inbounds {
		inbound, err := proxy.BuildInbound(conf.Protocol, conf.Setting)
		if err != nil {
			return nil, err
		}
		p.inbounds = append(p.inbounds, inbound)
	}

	for _, conf := range c.Outbounds {
		outbound, err := proxy.BuildOutbound(conf.Protocol, conf.Setting)
		if err != nil {
			return nil, err
		}
		tag := strings.ToLower(conf.Tag)
		if _, ok := p.outbounds[tag]; ok {
			return nil, errors.New("duplicated outbound tag: " + conf.Tag)
		}
		p.outbounds[tag] = outbound
	}

	// built-in outbound
	directOutbound, err := proxy.BuildOutbound(direct.Tag, nil)
	if err != nil {
		return nil, err
	}
	p.outbounds[direct.Tag] = directOutbound
	rejectOutbound, err := proxy.BuildOutbound(reject.Tag, nil)
	if err != nil {
		return nil, err
	}
	p.outbounds[reject.Tag] = rejectOutbound

	rt, err := router.NewRouter(c.Rules)
	if err != nil {
		return nil, err
	}
	p.router = rt
	return p, nil
}

func acceptConn(ctx context.Context, ib proxy.Inbound, ch chan<- proxy.Conn) {
	for {
		select {
		case <-ctx.Done():
			return
		case conn, ok := <-ib.Accept():
			if !ok {
				// inbound server closed
				return
			}
			ch <- conn
		}
	}
}

func (p *Proxy) Start(ctx context.Context) {
	connCh := make(chan proxy.Conn, 32)
	for _, inbound := range p.inbounds {
		go inbound.Serve()
		go acceptConn(ctx, inbound, connCh)
	}

	// cleanup if context ends
	defer func() {
		for _, inbound := range p.inbounds {
			_ = inbound.Close()
		}
		for _, outbound := range p.outbounds {
			if c, ok := outbound.(io.Closer); ok {
				c.Close()
			}
		}
		// drain the channel, prevent connections from being not closed
		close(connCh)
		for conn := range connCh {
			_ = conn.Close()
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case conn := <-connCh:
			go p.handleConnection(ctx, conn)
		}
	}
}

func (p *Proxy) handleConnection(ctx context.Context, inConn proxy.Conn) {
	activeConn.Inc()
	defer activeConn.Dec()

	defer inConn.Close()
	addr := inConn.DstAddr()
	tag := p.router.Dispatch(addr)
	outbound, ok := p.outbounds[tag]
	if !ok {
		log.Errorf("unknown outbound tag: %s", tag)
		return
	}
	glog.Printf("accept %s [%s]\n", addr, tag)

	dialCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	outConn, err := outbound.DialContext(dialCtx, addr)
	if err != nil {
		log.Errorf("dial %s err: %s", addr, err)
		return
	}
	defer outConn.Close()

	errCh := link(inConn, outConn)
	select {
	case <-ctx.Done():
	case err := <-errCh:
		if err != nil {
			// mostly idle timeout
			log.Warn(err)
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

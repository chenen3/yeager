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
	// ListenAndServe start the proxy server and block until closed or encounter error
	ListenAndServe(handle func(ctx context.Context, conn net.Conn, network, addr string)) error
	Close() error
}

type Outbounder interface {
	DialContext(ctx context.Context, network string, addr string) (net.Conn, error)
}

type Proxy struct {
	conf      *config.Config
	inbounds  []Inbounder
	outbounds map[string]Outbounder
	router    *route.Router
	done      chan struct{}
	nat       *nat // for UDP
}

func NewProxy(conf *config.Config) (*Proxy, error) {
	p := &Proxy{
		conf:      conf,
		outbounds: make(map[string]Outbounder, 2+len(conf.Outbounds)),
		done:      make(chan struct{}),
		nat:       newNAT(),
	}

	if conf.Inbounds.SOCKS != nil {
		srv, err := socks.NewTCPServer(conf.Inbounds.SOCKS)
		if err != nil {
			return nil, err
		}
		p.inbounds = append(p.inbounds, srv)
		udpSrv, err := socks.NewUDPServer(conf.Inbounds.SOCKS)
		if err != nil {
			return nil, err
		}
		p.inbounds = append(p.inbounds, udpSrv)
	}
	if conf.Inbounds.HTTP != nil {
		srv, err := http.NewServer(conf.Inbounds.HTTP)
		if err != nil {
			return nil, err
		}
		p.inbounds = append(p.inbounds, srv)
	}
	if conf.Inbounds.Yeager != nil {
		srv, err := yeager.NewServer(conf.Inbounds.Yeager)
		if err != nil {
			return nil, err
		}
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
		go func(ib Inbounder) {
			defer wg.Done()
			err := ib.ListenAndServe(p.handle)
			if err != nil {
				log.L().Error(err)
				// clean up before exit
				once.Do(func() {
					if err := p.Close(); err != nil {
						log.L().Error(err)
					}
				})
				return
			}
		}(inbound)
	}

	wg.Wait()
	<-p.done
	return nil
}

func (p *Proxy) Close() error {
	var err error
	defer close(p.done)
	for _, ib := range p.inbounds {
		e := ib.Close()
		if e != nil {
			err = e
		}
	}

	for _, outbound := range p.outbounds {
		if c, ok := outbound.(io.Closer); ok {
			e := c.Close()
			if e != nil {
				err = e
			}
		}
	}

	return err
}

var activeConn = expvar.NewInt("activeConn")

func (p *Proxy) handle(ctx context.Context, inConn net.Conn, network, addr string) {
	if p.conf.Develop {
		activeConn.Add(1)
		defer activeConn.Add(-1)
	}

	switch network {
	case "tcp":
		p.handleTCP(ctx, inConn, addr)
	case "udp":
		p.handleUDP(ctx, inConn, addr)
	default:
		log.L().Errorf("unknown network: %s", network)
		inConn.Close()
	}
}

func (p *Proxy) handleTCP(ctx context.Context, inConn net.Conn, addr string) {
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
	log.L().Infof("receive %s %s from %s, dispatch to [%s]", "tcp", addr, inConn.RemoteAddr(), tag)

	dialCtx, cancel := context.WithTimeout(ctx, common.DialTimeout)
	defer cancel()
	outConn, err := outbound.DialContext(dialCtx, "tcp", addr)
	if err != nil {
		log.L().Errorf("dial %s: %s", addr, err)
		return
	}
	defer outConn.Close()

	errCh := relayTCP(inConn, outConn)
	select {
	case <-ctx.Done():
	case err := <-errCh:
		if err != nil {
			log.L().Warnf("relayTCP %s: %s", addr, err)
		}
	}
}

func relayTCP(a, b io.ReadWriter) <-chan error {
	errCh := make(chan error, 2)
	go func() {
		_, err := io.Copy(a, b)
		errCh <- err
	}()
	go func() {
		_, err := io.Copy(b, a)
		errCh <- err
	}()
	return errCh
}

func (p *Proxy) handleUDP(ctx context.Context, inConn net.Conn, addr string) {
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
	log.L().Infof("receive %s %s from %s, dispatch to [%s]", "udp", addr, inConn.RemoteAddr(), tag)

	outConn, ok := p.nat.Get(inConn.RemoteAddr().String())
	if !ok {
		dialCtx, cancel := context.WithTimeout(ctx, common.DialTimeout)
		defer cancel()
		outConn, err = outbound.DialContext(dialCtx, "udp", addr)
		if err != nil {
			log.L().Errorf("dial %s: %s", addr, err)
			return
		}
		p.nat.Put(inConn.RemoteAddr().String(), outConn)
		// client <- inbound conn <- outbound conn <- remote
		go func() {
			defer outConn.Close()
			defer p.nat.Delete(inConn.RemoteAddr().String())
			// here inConn is socks.ServerUDPConn,
			// which still available even "closed"
			_, err := io.Copy(inConn, outConn)
			if err != nil {
				log.L().Warnf(err.Error())
			}
		}()
	}

	// client -> inbound conn -> outbound conn -> remote
	errCh := func() <-chan error {
		ch := make(chan error, 1)
		go func() {
			_, err = io.Copy(outConn, inConn)
			ch <- err
		}()
		return ch
	}()
	select {
	case <-ctx.Done():
	case err := <-errCh:
		if err != nil {
			log.L().Warnf("relayTCP %s: %s", addr, err)
		}
	}
}

type nat struct {
	m  map[string]net.Conn
	mu sync.RWMutex
}

func newNAT() *nat {
	return &nat{
		m: make(map[string]net.Conn),
	}
}

func (n *nat) Put(key string, conn net.Conn) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.m[key] = conn
}

func (n *nat) Get(key string) (net.Conn, bool) {
	n.mu.RLock()
	defer n.mu.RUnlock()
	conn, ok := n.m[key]
	return conn, ok
}

func (n *nat) Delete(key string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	delete(n.m, key)
}

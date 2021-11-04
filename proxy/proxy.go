package proxy

import (
	"context"
	"errors"
	"expvar"
	"io"
	"net"
	"strings"
	"sync"

	"go.uber.org/zap"

	"github.com/chenen3/yeager/config"
	"github.com/chenen3/yeager/proxy/common"
	"github.com/chenen3/yeager/proxy/direct"
	"github.com/chenen3/yeager/proxy/http"
	"github.com/chenen3/yeager/proxy/reject"
	"github.com/chenen3/yeager/proxy/socks"
	"github.com/chenen3/yeager/proxy/yeager"
	"github.com/chenen3/yeager/route"
)

type inbounder interface {
	// ListenAndServe start the proxy server and block until closed or encounter error
	ListenAndServe(handle func(ctx context.Context, conn net.Conn, addr string)) error
	Close() error
}

type outbounder interface {
	DialContext(ctx context.Context, addr string) (net.Conn, error)
}

type Proxy struct {
	conf      *config.Config
	inbounds  []inbounder
	outbounds map[string]outbounder
	router    *route.Router
	done      chan struct{}
}

func NewProxy(conf *config.Config) (*Proxy, error) {
	p := &Proxy{
		conf:      conf,
		outbounds: make(map[string]outbounder, 2+len(conf.Outbounds)),
		done:      make(chan struct{}),
	}

	if conf.Inbounds.SOCKS != nil {
		srv, err := socks.NewServer(conf.Inbounds.SOCKS)
		if err != nil {
			return nil, err
		}
		p.inbounds = append(p.inbounds, srv)
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
		go func(ib inbounder) {
			defer wg.Done()
			err := ib.ListenAndServe(p.handle)
			if err != nil {
				zap.S().Error(err)
				// clean up before exit
				once.Do(func() {
					if err := p.Close(); err != nil {
						zap.S().Error(err)
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

func (p *Proxy) handle(ctx context.Context, inConn net.Conn, addr string) {
	if p.conf.Develop {
		activeConn.Add(1)
		defer activeConn.Add(-1)
	}
	defer inConn.Close()

	tag, err := p.router.Dispatch(addr)
	if err != nil {
		zap.S().Error(err)
		return
	}
	outbound, ok := p.outbounds[tag]
	if !ok {
		zap.S().Errorf("unknown outbound tag: %s", tag)
		return
	}
	zap.L().Info("proxy request",
		zap.String("addr", addr),
		zap.String("src", inConn.RemoteAddr().String()),
		zap.String("dst", tag),
	)

	dialCtx, cancel := context.WithTimeout(ctx, common.DialTimeout)
	defer cancel()
	outConn, err := outbound.DialContext(dialCtx, addr)
	if err != nil {
		zap.S().Error(err)
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
			zap.S().Warnf("%s, dst %s", err, addr)
		}
	}
}

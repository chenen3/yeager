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
	"github.com/chenen3/yeager/proxy/direct"
	"github.com/chenen3/yeager/proxy/http"
	"github.com/chenen3/yeager/proxy/reject"
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

var activeConnCnt = expvar.NewInt("activeConn")

func (p *Proxy) handle(ctx context.Context, ibConn net.Conn, addr string) {
	if p.conf.Debug {
		activeConnCnt.Add(1)
		defer activeConnCnt.Add(-1)
	}

	tag, err := p.router.Dispatch(addr)
	if err != nil {
		log.Errorf("dispatch %s: %s", addr, err)
		return
	}
	outbound, ok := p.outbounds[tag]
	if !ok {
		log.Errorf("unknown outbound tag: %s", tag)
		return
	}
	if p.conf.Verbose {
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

	errCh := make(chan error, 1)
	relay(errCh, ibConn, obConn)
	select {
	case <-ctx.Done():
	case err := <-errCh:
		if err != nil {
			log.Errorf("relay %s: %s", addr, err)
			return
		}
	}
}

func relay(errCh chan<- error, ibConn, obConn net.Conn) {
	ibErrCh := make(chan error, 1)
	go func() {
		_, err := io.Copy(obConn, ibConn)
		if err != nil {
			err = errors.New("copy inbound->outbound: " + err.Error())
			// unblock future read on obConn
			obConn.SetReadDeadline(time.Now().Add(5 * time.Second))
		}
		ibErrCh <- err
	}()
	_, obErr := io.Copy(ibConn, obConn)
	if obErr != nil {
		obErr = errors.New("copy outbound->inbound: " + obErr.Error())
		// unblock future read on ibConn
		ibConn.SetReadDeadline(time.Now().Add(5 * time.Second))
	}
	ibErr := <-ibErrCh

	var err error
	if ibErr != nil && ibErr != os.ErrDeadlineExceeded {
		err = ibErr
	} else if obErr != nil && obErr != os.ErrDeadlineExceeded {
		err = obErr
	}
	errCh <- err
}

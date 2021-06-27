package yeager

import (
	"context"
	"errors"
	"io"
	glog "log"
	"strings"
	"time"

	"yeager/config"
	"yeager/log"
	"yeager/protocol"
	"yeager/protocol/direct"
	_ "yeager/protocol/http"
	"yeager/protocol/reject"
	_ "yeager/protocol/socks"
	_ "yeager/protocol/yeager"
	"yeager/router"
)

type Proxy struct {
	inbounds  []protocol.Inbound
	outbounds map[string]protocol.Outbound
	router    *router.Router
}

func NewProxy(c config.Config) (*Proxy, error) {
	p := &Proxy{
		outbounds: make(map[string]protocol.Outbound, 2+len(c.Outbounds)),
	}
	for _, conf := range c.Inbounds {
		inbound, err := protocol.BuildInbound(conf.Protocol, conf.Setting)
		if err != nil {
			return nil, err
		}
		p.inbounds = append(p.inbounds, inbound)
	}

	for _, conf := range c.Outbounds {
		outbound, err := protocol.BuildOutbound(conf.Protocol, conf.Setting)
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
	directOutbound, err := protocol.BuildOutbound(direct.Tag, nil)
	if err != nil {
		return nil, err
	}
	p.outbounds[direct.Tag] = directOutbound
	rejectOutbound, err := protocol.BuildOutbound(reject.Tag, nil)
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

func acceptConn(ctx context.Context, ib protocol.Inbound, ch chan<- protocol.Conn) {
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
	connCh := make(chan protocol.Conn, 32)
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

func (p *Proxy) handleConnection(ctx context.Context, inConn protocol.Conn) {
	defer inConn.Close()
	addr := inConn.DstAddr()
	tag := p.router.Dispatch(addr)
	outbound, ok := p.outbounds[tag]
	if !ok {
		log.Errorf("unknown outbound tag: %s", tag)
		return
	}
	// glog.Printf("accepted %s [%s]", addr, tag)

	dialCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	start := time.Now()
	outConn, err := outbound.DialContext(dialCtx, addr)
	if err != nil {
		log.Error(err)
		return
	}
	defer outConn.Close()
	glog.Printf("accepted %s [%s], dial %dms", addr, tag, time.Since(start).Milliseconds())

	errCh := link(inConn, outConn)
	select {
	case <-ctx.Done():
	case err := <-errCh:
		if err != nil {
			log.Error(err)
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

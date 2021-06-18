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
	"yeager/util"
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
	glog.Printf("accepted %s [%s]\n", addr, tag)

	outConn, err := outbound.Dial(addr)
	if err != nil {
		log.Error(err)
		return
	}
	defer outConn.Close()

	iConn := util.ConnWithIdleTimeout(inConn, 5*time.Minute)
	oConn := util.ConnWithIdleTimeout(outConn, 5*time.Minute)

	// bidirectional connection, one of which closed, the other shall close immediately
	errCh := make(chan error, 2)
	go copyConn(oConn, iConn, errCh)
	go copyConn(iConn, oConn, errCh)
	select {
	case <-ctx.Done():
		return
	case err := <-errCh:
		if err != nil {
			log.Error(err)
		}
	}
}

func copyConn(dst io.Writer, src io.Reader, errCh chan<- error) {
	_, err := io.Copy(dst, src)
	errCh <- err
}

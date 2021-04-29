package yeager

import (
	"context"
	"errors"
	"io"
	glog "log"
	"strings"

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
		outbounds: make(map[string]protocol.Outbound, 3),
	}
	for _, inboundConf := range c.Inbounds {
		inbound, err := protocol.BuildInbound(inboundConf.Protocol, inboundConf.Setting)
		if err != nil {
			return nil, err
		}
		p.inbounds = append(p.inbounds, inbound)
	}

	for _, outboundConf := range c.Outbounds {
		outbound, err := protocol.BuildOutbound(outboundConf.Protocol, outboundConf.Setting)
		if err != nil {
			return nil, err
		}
		tag := strings.ToLower(outboundConf.Tag)
		if _, ok := p.outbounds[tag]; ok {
			return nil, errors.New("duplicated outbound tag: " + outboundConf.Tag)
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

	router_, err := router.NewRouter(c.Rules)
	if err != nil {
		return nil, err
	}
	p.router = router_
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
			go p.handleConnection(conn)
		}
	}
}

func (p *Proxy) handleConnection(inConn protocol.Conn) {
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
		if err != reject.Err {
			log.Error(err)
		}
		return
	}
	defer outConn.Close()

	go io.Copy(outConn, inConn)
	io.Copy(inConn, outConn)
}

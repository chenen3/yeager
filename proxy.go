package yeager

import (
	"context"
	"io"
	glog "log"

	"yeager/config"
	"yeager/log"
	"yeager/protocol"
	_ "yeager/protocol/direct"
	_ "yeager/protocol/http"
	"yeager/protocol/reject"
	_ "yeager/protocol/socks"
	_ "yeager/protocol/yeager"
	"yeager/router"
)

type Proxy struct {
	inbounds  []protocol.Inbound
	outbounds map[router.PolicyType]protocol.Outbound
	router    *router.Router
}

func NewProxy(c config.Config) (*Proxy, error) {
	p := &Proxy{
		outbounds: make(map[router.PolicyType]protocol.Outbound, 3),
	}
	for _, inboundConf := range c.Inbounds {
		inbound, err := protocol.BuildInbound(inboundConf.Protocol, inboundConf.Setting)
		if err != nil {
			return nil, err
		}
		p.inbounds = append(p.inbounds, inbound)
	}

	if c.Outbound.Protocol != "" {
		outbound, err := protocol.BuildOutbound(c.Outbound.Protocol, c.Outbound.Setting)
		if err != nil {
			return nil, err
		}
		p.outbounds[router.PolicyProxy] = outbound
	}

	// built-in proxy policy: direct and reject
	directOutbound, err := protocol.BuildOutbound("direct", nil)
	if err != nil {
		return nil, err
	}
	p.outbounds[router.PolicyDirect] = directOutbound
	rejectOutbound, err := protocol.BuildOutbound("reject", nil)
	if err != nil {
		return nil, err
	}
	p.outbounds[router.PolicyReject] = rejectOutbound

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
	policy := p.router.Dispatch(addr)
	outbound, ok := p.outbounds[policy]
	if !ok {
		log.Errorf("unknown outbound policy: %s", policy)
		return
	}
	glog.Printf("accepted %s [%s]\n", addr, policy)
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

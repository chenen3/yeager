package main

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

func NewProxy(c config.Config) (*Proxy, error) {
	ctx, cancel := context.WithCancel(context.Background())
	p := &Proxy{
		ctx:       ctx,
		cancel:    cancel,
		outbounds: make(map[router.PolicyType]protocol.Outbound, 3),
	}
	for _, inboundConf := range c.Inbounds {
		inbound, err := protocol.BuildInbound(inboundConf)
		if err != nil {
			return nil, err
		}
		p.inbounds = append(p.inbounds, inbound)
	}

	if c.Outbound.Protocol != "" {
		outbound, err := protocol.BuildOutbound(c.Outbound)
		if err != nil {
			return nil, err
		}
		p.outbounds[router.PolicyProxy] = outbound
	}

	// built-in proxy policy: direct and reject
	directOutbound, err := protocol.BuildOutbound(config.Proto{Protocol: "direct"})
	if err != nil {
		return nil, err
	}
	p.outbounds[router.PolicyDirect] = directOutbound
	rejectOutbound, err := protocol.BuildOutbound(config.Proto{Protocol: "reject"})
	if err != nil {
		return nil, err
	}
	p.outbounds[router.PolicyReject] = rejectOutbound

	router, err := router.NewRouter(c.Rules)
	if err != nil {
		return nil, err
	}
	p.router = router
	return p, nil
}

type Proxy struct {
	inbounds  []protocol.Inbound
	outbounds map[router.PolicyType]protocol.Outbound
	router    *router.Router
	ctx       context.Context
	cancel    context.CancelFunc
}

func (p *Proxy) Start() error {
	// TODO 信道长度，keep-alive
	connCh := make(chan protocol.Conn, 32)
	for _, inbound := range p.inbounds {
		go func(inbound protocol.Inbound, connCh chan<- protocol.Conn) {
			for {
				conn, ok := <-inbound.Accept()
				if !ok {
					// inbound server closed
					return
				}
				connCh <- conn
			}
		}(inbound, connCh)
	}

	for {
		select {
		case conn := <-connCh:
			go p.handleConnection(conn)
		case <-p.ctx.Done():
			// in case connections left unhandled
			select {
			case conn := <-connCh:
				conn.Close()
			default:
				return p.ctx.Err()
			}
		}
	}
}

func (p *Proxy) handleConnection(inConn protocol.Conn) {
	defer inConn.Close()
	policy := p.router.Dispatch(inConn.DstAddr())
	outbound, ok := p.outbounds[policy]
	if !ok {
		log.Errorf("unknown outbound proxy policy: %s", policy)
		return
	}
	glog.Printf("accepted %s [%s]\n", inConn.DstAddr(), policy)
	outConn, err := outbound.Dial(inConn.DstAddr())
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

func (p *Proxy) Close() error {
	for _, inbound := range p.inbounds {
		inbound.Close()
	}
	p.cancel()
	return nil
}

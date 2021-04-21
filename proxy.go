package main

import (
	"context"
	"io"
	glog "log"

	"github.com/opentracing/opentracing-go"
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

	router_, err := router.NewRouter(c.Rules)
	if err != nil {
		return nil, err
	}
	p.router = router_
	return p, nil
}

type Proxy struct {
	inbounds  []protocol.Inbound
	outbounds map[router.PolicyType]protocol.Outbound
	router    *router.Router
	ctx       context.Context
	cancel    context.CancelFunc
}

func (p *Proxy) Start() {
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
				return
			}
		}
	}
}

func (p *Proxy) handleConnection(inConn protocol.Conn) {
	defer inConn.Close()
	addr := inConn.DstAddr()
	span := opentracing.StartSpan("dispatch")
	span.SetTag("addr", addr.String())
	ctx := opentracing.ContextWithSpan(context.Background(), span)

	policy := p.router.Dispatch(ctx, addr)
	outbound, ok := p.outbounds[policy]
	if !ok {
		log.Errorf("unknown outbound policy: %s", policy)
		span.Finish()
		return
	}
	glog.Printf("accepted %s [%s]\n", addr, policy)
	outConn, err := outbound.Dial(ctx, addr)
	span.Finish()
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

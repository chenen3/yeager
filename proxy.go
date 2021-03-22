package main

import (
	"context"
	"errors"
	"io"

	"yeager/config"
	"yeager/log"
	"yeager/protocol"
	_ "yeager/protocol/freedom"
	_ "yeager/protocol/http"
	_ "yeager/protocol/socks"
	_ "yeager/protocol/yeager"
)

func NewProxy(c config.Config) (*Proxy, error) {
	ctx, cancel := context.WithCancel(context.Background())
	p := &Proxy{
		ctx:    ctx,
		cancel: cancel,
	}
	for _, inboundConf := range c.Inbounds {
		buildInbound, ok := protocol.InboundBuilder(inboundConf.Protocol)
		if !ok {
			return nil, errors.New("unknown protocol: " + inboundConf.Protocol)
		}
		inbound, err := buildInbound(inboundConf.Setting)
		if err != nil {
			return nil, err
		}
		p.inbounds = append(p.inbounds, inbound)
	}

	buildOutbound, ok := protocol.OutboundBuilder(c.Outbound.Protocol)
	if !ok {
		return nil, errors.New("unknown protocol: " + c.Outbound.Protocol)
	}
	var err error
	p.outbound, err = buildOutbound(c.Outbound.Setting)
	if err != nil {
		return nil, err
	}
	return p, nil
}

type Proxy struct {
	inbounds []protocol.Inbound
	outbound protocol.Outbound
	ctx      context.Context
	cancel   context.CancelFunc
}

func (p *Proxy) Start() error {
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
	outConn, err := p.outbound.Dial(inConn.DstAddr())
	if err != nil {
		log.Error(err)
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

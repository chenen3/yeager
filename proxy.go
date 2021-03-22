package main

import (
	"context"
	"errors"
	"io"

	"yeager/config"
	"yeager/log"
	"yeager/protocol"
	_ "yeager/protocol/freedom"
	_ "yeager/protocol/socks"
	_ "yeager/protocol/yeager"
)

func NewProxy(c config.Config) (*Proxy, error) {
	ctx, cancel := context.WithCancel(context.Background())
	p := &Proxy{
		ctx:    ctx,
		cancel: cancel,
	}
	buildInbound, ok := protocol.InboundBuilder(c.Inbound.Protocol)
	if !ok {
		return nil, errors.New("unknown protocol: " + c.Inbound.Protocol)
	}
	var err error
	p.inbound, err = buildInbound(c.Inbound.Setting)
	if err != nil {
		return nil, err
	}

	buildOutbound, ok := protocol.OutboundBuilder(c.Outbound.Protocol)
	if !ok {
		return nil, errors.New("unknown protocol: " + c.Inbound.Protocol)
	}
	p.outbound, err = buildOutbound(c.Outbound.Setting)
	if err != nil {
		return nil, err
	}
	return p, nil
}

type Proxy struct {
	inbound  protocol.Inbound
	outbound protocol.Outbound
	ctx      context.Context
	cancel   context.CancelFunc
}

func (p *Proxy) Start() error {
	for {
		inConn, err := p.inbound.Accept()
		if err != nil {
			return err
		}
		go p.handleConnection(inConn)
	}
}

func ioCopy(dst io.Writer, src io.Reader) {
	_, err := io.Copy(dst, src)
	if err != nil {
		log.Error(err)
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

	go ioCopy(outConn, inConn)
	ioCopy(inConn, outConn)
	return
}

func (p *Proxy) Close() error {
	p.inbound.Close()
	p.cancel()
	return nil
}

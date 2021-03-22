package protocol

import (
	"encoding/json"
	"io"
	"net"
)

// Conn is the interface that wrap net.Conn with destination address method
type Conn interface {
	net.Conn
	DstAddr() net.Addr
}

type Inbound interface {
	// TODO: Start() ?
	Accept() <-chan Conn
	io.Closer
}

type Outbound interface {
	Dial(dstAddr net.Addr) (net.Conn, error)
}

// 不直接引用配置结构体，是为了让"协议模块"与"配置模块"解耦
type InboundBuilderFunc func(setting json.RawMessage) (Inbound, error)

var inboundBuilders = make(map[string]InboundBuilderFunc)

func RegisterInboundBuilder(name string, b InboundBuilderFunc) {
	inboundBuilders[name] = b
}

func InboundBuilder(name string) (InboundBuilderFunc, bool) {
	b, ok := inboundBuilders[name]
	return b, ok
}

// 不直接引用配置结构体，是为了让"协议模块"与"配置模块"解耦
type OutboundBuilderFunc func(setting json.RawMessage) (Outbound, error)

var outboundBuilders = make(map[string]OutboundBuilderFunc)

func RegisterOutboundBuilder(name string, b OutboundBuilderFunc) {
	outboundBuilders[name] = b
}

func OutboundBuilder(name string) (OutboundBuilderFunc, bool) {
	b, ok := outboundBuilders[name]
	return b, ok
}

// Connection is an implementation of the Conn interface
type Connection struct {
	net.Conn
	dstAddr net.Addr
}

func NewConn(conn net.Conn, dstAddr net.Addr) *Connection {
	return &Connection{Conn: conn, dstAddr: dstAddr}
}

func (c *Connection) DstAddr() net.Addr {
	return c.dstAddr
}

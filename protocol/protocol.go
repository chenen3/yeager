package protocol

import (
	"encoding/json"
	"io"
	"net"
)

type Conn interface {
	net.Conn
	DstAddr() net.Addr // 入站连接的目标地址
}

type Inbound interface {
	Accept() (Conn, error)
	io.Closer
}

type Outbound interface {
	// Dial 接收入站连接的目标地址 dstAddr 作为参数
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

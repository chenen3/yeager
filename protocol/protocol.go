package protocol

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"strconv"
)

// Conn is the interface that wrap net.Conn with destination address method
type Conn interface {
	net.Conn
	DstAddr() *Address
}

type Inbound interface {
	Accept() <-chan Conn
	io.Closer
}

type Outbound interface {
	Dial(dstAddr *Address) (net.Conn, error)
}

type InboundBuilderFunc func(setting json.RawMessage) (Inbound, error)

var inboundBuilders = make(map[string]InboundBuilderFunc)

func RegisterInboundBuilder(name string, b InboundBuilderFunc) {
	inboundBuilders[name] = b
}

func BuildInbound(proto string, conf json.RawMessage) (Inbound, error) {
	build, ok := inboundBuilders[proto]
	if !ok {
		return nil, errors.New("unknown protocol: " + proto)
	}
	return build(conf)
}

type OutboundBuilderFunc func(setting json.RawMessage) (Outbound, error)

var outboundBuilders = make(map[string]OutboundBuilderFunc)

func RegisterOutboundBuilder(name string, b OutboundBuilderFunc) {
	outboundBuilders[name] = b
}

func BuildOutbound(proto string, conf json.RawMessage) (Outbound, error) {
	build, ok := outboundBuilders[proto]
	if !ok {
		return nil, errors.New("unknown protocol: " + proto)
	}
	return build(conf)
}

// Connection implements the Conn interface
type Connection struct {
	net.Conn
	dstAddr *Address
}

func NewConn(conn net.Conn, dstAddr *Address) *Connection {
	return &Connection{Conn: conn, dstAddr: dstAddr}
}

func (c *Connection) DstAddr() *Address {
	return c.dstAddr
}

type AddrType int

const (
	AddrIPv4 = iota
	AddrIPv6
	AddrDomainName
)

type Address struct {
	Type AddrType
	Host string
	Port int
	IP   net.IP
}

func NewAddress(host string, port int) *Address {
	var at AddrType
	ip := net.ParseIP(host)
	if ip == nil {
		at = AddrDomainName
	} else if ipv4 := ip.To4(); ipv4 != nil {
		at = AddrIPv4
		ip = ipv4
	} else {
		at = AddrIPv6
		ip = ip.To16()
	}

	return &Address{
		Type: at,
		Host: host,
		Port: port,
		IP:   ip,
	}
}

func (a *Address) Network() string {
	return "tcp"
}

func (a *Address) String() string {
	return net.JoinHostPort(a.Host, strconv.Itoa(a.Port))
}

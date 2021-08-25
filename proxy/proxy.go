package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"strconv"
	"time"
)

const (
	DialTimeout      = 4 * time.Second
	HandshakeTimeout = 5 * time.Second
	IdleConnTimeout  = 5 * time.Minute
)

type Inbound interface {
	// ListenAndServe start the proxy server and block until context closed or encounter error
	ListenAndServe(context.Context) error
	// RegisterHandler register handler for income connection
	RegisterHandler(func(context.Context, net.Conn, *Address))
}

type Outbound interface {
	DialContext(ctx context.Context, addr *Address) (net.Conn, error)
}

type inboundBuilder func(setting json.RawMessage) (Inbound, error)

var inboundBuilders = make(map[string]inboundBuilder)

func RegisterInboundBuilder(name string, b inboundBuilder) {
	inboundBuilders[name] = b
}

func BuildInbound(proto string, conf json.RawMessage) (Inbound, error) {
	build, ok := inboundBuilders[proto]
	if !ok {
		return nil, errors.New("unknown protocol: " + proto)
	}
	return build(conf)
}

type outboundBuilder func(setting json.RawMessage) (Outbound, error)

var outboundBuilders = make(map[string]outboundBuilder)

func RegisterOutboundBuilder(name string, b outboundBuilder) {
	outboundBuilders[name] = b
}

func BuildOutbound(proto string, conf json.RawMessage) (Outbound, error) {
	build, ok := outboundBuilders[proto]
	if !ok {
		return nil, errors.New("unknown protocol: " + proto)
	}
	return build(conf)
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

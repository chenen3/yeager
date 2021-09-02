package proxy

import (
	"context"
	"net"
	"strconv"
	"time"
)

const (
	DialTimeout      = 4 * time.Second
	HandshakeTimeout = 5 * time.Second
	IdleConnTimeout  = 5 * time.Minute
)

type Handler func(context.Context, net.Conn, *Address)

type Inbound interface {
	// ListenAndServe start the proxy server and block until closed or encounter error
	ListenAndServe(Handler) error
	Close() error
}

type Outbound interface {
	DialContext(ctx context.Context, addr *Address) (net.Conn, error)
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

package proxy

import (
	"context"
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

type Handler func(ctx context.Context, conn net.Conn, addr string)

type Inbound interface {
	// ListenAndServe start the proxy server and block until closed or encounter error
	ListenAndServe(Handler) error
	Close() error
}

type Outbound interface {
	DialContext(ctx context.Context, addr string) (net.Conn, error)
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

// ParseAddress parse a network address to domain, ip
func ParseAddress(addr string) (*Address, error) {
	host, rawport, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	port, err := strconv.Atoi(rawport)
	if err != nil {
		return nil, err
	}
	if 0 > port || port > 65535 {
		return nil, errors.New("invalid port")
	}

	var typ AddrType
	ip := net.ParseIP(host)
	if ip == nil {
		typ = AddrDomainName
	} else if ipv4 := ip.To4(); ipv4 != nil {
		typ = AddrIPv4
		ip = ipv4
	} else {
		typ = AddrIPv6
		ip = ip.To16()
	}

	a := &Address{
		Type: typ,
		Host: host,
		Port: port,
		IP:   ip,
	}
	return a, nil
}

func (a *Address) Network() string {
	return "tcp"
}

func (a *Address) String() string {
	return net.JoinHostPort(a.Host, strconv.Itoa(a.Port))
}

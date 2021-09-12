package proxy

import (
	"context"
	"errors"
	"net"
	"strconv"
	"time"
)

const (
	DialTimeout       = 4 * time.Second
	HandshakeTimeout  = 5 * time.Second
	MaxConnectionIdle = 5 * time.Minute
)

type Handler func(ctx context.Context, conn net.Conn, addr *Address)

type Inbound interface {
	// ListenAndServe start the proxy server and block until closed or encounter error
	ListenAndServe(Handler) error
	Close() error
}

type Outbound interface {
	DialContext(ctx context.Context, network, addr string) (net.Conn, error)
}

type AddrType int

const (
	AddrIPv4 = iota
	AddrIPv6
	AddrDomainName
)

type Address struct {
	network string
	Type    AddrType
	Host    string
	Port    int
	IP      net.IP
}

func (a *Address) Network() string {
	return a.network
}

func (a *Address) String() string {
	return net.JoinHostPort(a.Host, strconv.Itoa(a.Port))
}

// ParseAddress parse a network address to domain, ip
func ParseAddress(network, addr string) (*Address, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	if host == "" {
		host = "0.0.0.0"
	}

	uintPort, err := strconv.ParseUint(port, 10, 16)
	if err != nil {
		return nil, errors.New("failed to parse port: " + err.Error())
	}

	return ParseHostPort(network, host, int(uintPort))
}

func ParseHostPort(network, host string, port int) (*Address, error) {
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
		network: network,
		Type:    typ,
		Host:    host,
		Port:    port,
		IP:      ip,
	}
	return a, nil
}

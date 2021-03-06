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
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	uintPort, err := strconv.ParseUint(port, 10, 16)
	if err != nil {
		return nil, errors.New("failed to parse port: " + err.Error())
	}
	portnum := int(uintPort)

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
		Port: portnum,
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

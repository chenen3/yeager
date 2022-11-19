package util

import (
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

// AllocatePort allocates an available port
func AllocatePort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	ln.Close()
	return ln.Addr().(*net.TCPAddr).Port, nil
}

// Addr type
const (
	AddrIPv4 = iota
	AddrIPv6
	AddrDomainName
)

type Addr struct {
	network string
	Type    int
	Host    string
	Port    int
	IP      net.IP
}

// ParseAddr parse a network address to domain, ip
func ParseAddr(network, addr string) (*Addr, error) {
	if network != "tcp" {
		return nil, errors.New("unsupported network: " + network)
	}

	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	if host == "" {
		host = "0.0.0.0"
	}

	portUint, err := strconv.ParseUint(portStr, 10, 16)
	if err != nil {
		return nil, errors.New("failed to parse port: " + err.Error())
	}
	port := int(portUint)

	var addrType int
	ip := net.ParseIP(host)
	if ip == nil {
		addrType = AddrDomainName
	} else if ipv4 := ip.To4(); ipv4 != nil {
		addrType = AddrIPv4
		ip = ipv4
	} else {
		addrType = AddrIPv6
		ip = ip.To16()
	}

	a := &Addr{
		network: network,
		Type:    addrType,
		Host:    host,
		Port:    port,
		IP:      ip,
	}
	return a, nil
}

func (a *Addr) Network() string {
	return a.network
}

func (a *Addr) String() string {
	return net.JoinHostPort(a.Host, strconv.Itoa(a.Port))
}

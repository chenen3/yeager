package util

import (
	"errors"
	"net"
	"strconv"
)

// ChoosePort choose a local port number automatically
func ChoosePort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	ln.Close()
	return ln.Addr().(*net.TCPAddr).Port, nil
}

// compatible with SOCKS5 protocol address type
const (
	AtypIPv4   = 0x01
	AtypDomain = 0x03
	AtypIPv6   = 0x04
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
	portnum := int(uintPort)

	var typ int
	ip := net.ParseIP(host)
	if ip == nil {
		typ = AtypDomain
	} else if ipv4 := ip.To4(); ipv4 != nil {
		typ = AtypIPv4
		ip = ipv4
	} else {
		typ = AtypIPv6
		ip = ip.To16()
	}

	a := &Addr{
		network: network,
		Type:    typ,
		Host:    host,
		Port:    portnum,
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

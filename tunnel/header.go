package tunnel

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"time"
)

const maxAddrLen = 1 + 1 + 255 + 2

// addr type
const (
	addrIPv4   = 0x01
	addrDomain = 0x03
	addrIPv6   = 0x04
)

type addr struct {
	Type uint
	Host string // for example: localhost, 127.0.0.1
	Port uint16
	IP   net.IP
}

func parseAddr(hostport string) (*addr, error) {
	host, portStr, err := net.SplitHostPort(hostport)
	if err != nil {
		return nil, err
	}
	if host == "" {
		host = "0.0.0.0"
	}

	port, err := strconv.ParseUint(portStr, 10, 16)
	if err != nil {
		return nil, errors.New("parse port: " + err.Error())
	}

	var addrType uint
	ip := net.ParseIP(host)
	if ip == nil {
		addrType = addrDomain
	} else if ipv4 := ip.To4(); ipv4 != nil {
		addrType = addrIPv4
		ip = ipv4
	} else {
		addrType = addrIPv6
		ip = ip.To16()
	}

	a := &addr{
		Type: addrType,
		Host: host,
		Port: uint16(port),
		IP:   ip,
	}
	return a, nil
}

// Deprecated: very obvious behavior, make it easy to detect
func WriteHeader(w io.Writer, hostport string) error {
	addr, err := parseAddr(hostport)
	if err != nil {
		return err
	}

	/*
		客户端请求格式，仿照socks5协议(以字节为单位):
		VER ATYP DST.ADDR DST.PORT
		1   1    动态     2
	*/
	b := make([]byte, 0, 1+maxAddrLen)
	// keep version number for backward compatibility
	b = append(b, 0x00)

	switch addr.Type {
	case addrIPv4:
		b = append(b, addrIPv4)
		b = append(b, addr.IP...)
	case addrDomain:
		b = append(b, addrDomain)
		b = append(b, byte(len(addr.Host)))
		b = append(b, []byte(addr.Host)...)
	case addrIPv6:
		b = append(b, addrIPv6)
		b = append(b, addr.IP...)
	default:
		return errors.New("bad address: " + hostport)
	}

	p := make([]byte, 2)
	binary.BigEndian.PutUint16(p, addr.Port)
	b = append(b, p...)

	_, err = w.Write(b)
	return err
}

// Deprecated: very obvious behavior, make it easy to detect
func ReadHeader(r io.Reader) (hostport string, err error) {
	/*
		客户端请求格式，仿照socks5协议(以字节为单位):
		VER ATYP DST.ADDR DST.PORT
		1   1    动态     2
	*/
	b := make([]byte, 1+maxAddrLen)
	if _, err = io.ReadFull(r, b[:2]); err != nil {
		return "", err
	}

	atyp := b[1]
	var host string
	switch atyp {
	case addrIPv4:
		if _, err = io.ReadFull(r, b[:net.IPv4len]); err != nil {
			return "", err
		}
		host = net.IPv4(b[0], b[1], b[2], b[3]).String()
	case addrDomain:
		if _, err = io.ReadFull(r, b[:1]); err != nil {
			return "", err
		}
		domainLen := b[0]
		if _, err = io.ReadFull(r, b[:domainLen]); err != nil {
			return "", err
		}
		host = string(b[:domainLen])
	case addrIPv6:
		if _, err = io.ReadFull(r, b[:net.IPv6len]); err != nil {
			return "", err
		}
		ipv6 := make(net.IP, net.IPv6len)
		copy(ipv6, b[:net.IPv6len])
		host = ipv6.String()
	default:
		return "", fmt.Errorf("unsupported address type: %x", atyp)
	}

	if _, err = io.ReadFull(r, b[:2]); err != nil {
		return "", err
	}
	port := binary.BigEndian.Uint16(b[:2])
	hostport = net.JoinHostPort(host, strconv.FormatUint(uint64(port), 10))
	return hostport, nil
}

// Deprecated
func TimeReadHeader(r io.Reader, timeout time.Duration) (addr string, err error) {
	type result struct {
		addr string
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		addr, err := ReadHeader(r)
		ch <- result{addr, err}
	}()

	t := time.NewTimer(timeout)
	select {
	case <-t.C:
		return "", errors.New("timeout")
	case r := <-ch:
		t.Stop()
		return r.addr, r.err
	}
}

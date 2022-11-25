package tunnel

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"time"

	ynet "github.com/chenen3/yeager/net"
)

const maxAddrLen = 1 + 1 + 255 + 2

const (
	addrIPv4   = 0x01
	addrDomain = 0x03
	addrIPv6   = 0x04
)

// TODO: rename API
// func ReadHeader(r io.Reader) (addr string, err error){}
// func WriteHeader(w io.Writer, addr string) {}

// MakeHeader 构造 header，包含目的地址
func MakeHeader(addr string) ([]byte, error) {
	dstAddr, err := ynet.ParseAddr("tcp", addr)
	if err != nil {
		return nil, err
	}

	/*
		客户端请求格式，仿照socks5协议(以字节为单位):
		VER ATYP DST.ADDR DST.PORT
		1   1    动态     2
	*/
	b := make([]byte, 0, 1+maxAddrLen)
	// keep version number for backward compatibility
	b = append(b, 0x00)

	switch dstAddr.Type {
	case ynet.AddrIPv4:
		b = append(b, addrIPv4)
		b = append(b, dstAddr.IP...)
	case ynet.AddrDomainName:
		b = append(b, addrDomain)
		b = append(b, byte(len(dstAddr.Host)))
		b = append(b, []byte(dstAddr.Host)...)
	case ynet.AddrIPv6:
		b = append(b, addrIPv6)
		b = append(b, dstAddr.IP...)
	default:
		return nil, errors.New("unsupported address type: " + dstAddr.String())
	}

	p := make([]byte, 2)
	binary.BigEndian.PutUint16(p, uint16(dstAddr.Port))
	b = append(b, p...)
	return b, nil
}

// ReadHeader 读取 header, 解析其目的地址
func ReadHeader(r io.Reader) (addr string, err error) {
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
	addr = net.JoinHostPort(host, strconv.Itoa(int(port)))
	return addr, nil
}

func TimeReadHeader(r io.Reader, d time.Duration) (addr string, err error) {
	type result struct {
		addr string
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		addr, err := ReadHeader(r)
		ch <- result{addr, err}
	}()

	t := time.NewTimer(d)
	select {
	case <-t.C:
		return "", errors.New("timeout")
	case r := <-ch:
		t.Stop()
		return r.addr, r.err
	}
}

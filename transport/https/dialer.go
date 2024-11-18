package https

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/http"
)

// dialer establish a tunnel with HTTP CONNECT.
type dialer struct {
	proxyAddr string
}

func NewDialer(proxyAddr string) *dialer {
	return &dialer{proxyAddr: proxyAddr}
}

func (d *dialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	proxyConn, err := net.Dial("tcp", d.proxyAddr)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(http.MethodConnect, "http://"+addr, nil)
	if err != nil {
		return nil, err
	}
	req.Host = addr

	err = req.Write(proxyConn)
	if err != nil {
		proxyConn.Close()
		return nil, err
	}

	resp, err := http.ReadResponse(bufio.NewReader(proxyConn), req)
	if err != nil {
		proxyConn.Close()
		return nil, err
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		proxyConn.Close()
		return nil, fmt.Errorf("proxy connection failed with status code %d", resp.StatusCode)
	}
	return proxyConn, nil
}

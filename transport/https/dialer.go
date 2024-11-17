package https

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/http"

	"github.com/chenen3/yeager/transport"
)

// StreamDialer establish a tunnel with HTTP CONNECT.
type StreamDialer struct {
	HostPort string
}

// Dial dials a TCP connection using the tunnel.
func (d *StreamDialer) Dial(ctx context.Context, addr string) (transport.Stream, error) {
	proxyConn, err := net.Dial("tcp", d.HostPort)
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
	return proxyConn.(*net.TCPConn), nil
}

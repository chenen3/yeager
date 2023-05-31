package http2

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"golang.org/x/net/http2"
)

type TunnelClient struct {
	addr     string
	tlsConf  *tls.Config
	mu       sync.Mutex
	clients  map[string]*http.Client
	connStat map[string]int
}

func NewTunnelClient(addr string, tlsConf *tls.Config) *TunnelClient {
	return &TunnelClient{
		addr:     addr,
		tlsConf:  tlsConf,
		clients:  make(map[string]*http.Client),
		connStat: make(map[string]int),
	}
}

func (c *TunnelClient) client(dst string) *http.Client {
	c.mu.Lock()
	defer c.mu.Unlock()
	client, ok := c.clients[dst]
	if ok {
		return client
	}

	transport := &http2.Transport{
		DialTLSContext: func(ctx context.Context, network, addr string, _ *tls.Config) (net.Conn, error) {
			d := &net.Dialer{
				Timeout:   5 * time.Second,
				KeepAlive: 30 * time.Second,
			}
			return tls.DialWithDialer(d, "tcp", addr, c.tlsConf)
		},
	}
	client = &http.Client{Transport: transport}
	c.clients[c.addr] = client
	return client
}

func (c *TunnelClient) trackConn(dst string, add bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if add {
		c.connStat[dst]++
	} else {
		c.connStat[dst]--
		if c.connStat[dst] == 0 {
			delete(c.connStat, dst)
			delete(c.clients, dst)
		}
	}
}

func (c *TunnelClient) DialContext(ctx context.Context, dst string) (io.ReadWriteCloser, error) {
	pr, pw := io.Pipe()
	req := &http.Request{
		Method:        "CONNECT",
		URL:           &url.URL{Scheme: "https", Host: c.addr},
		Proto:         "HTTP/2",
		ProtoMajor:    2,
		ProtoMinor:    0,
		Header:        make(http.Header),
		Body:          pr,
		ContentLength: -1,
	}
	req.Header.Add("dst", dst)
	req.Header.Set("User-Agent", "Chrome/76.0.3809.100")

	resp, err := c.client(dst).Do(req)
	if err != nil {
		return nil, errors.New("h2 request: " + err.Error())
	}
	c.trackConn(dst, true)

	rwc := &rwc{
		respBody:  resp.Body,
		reqBodyPW: pw,
		onclose: func() {
			c.trackConn(dst, false)
		},
	}
	if resp.StatusCode != http.StatusOK {
		rwc.Close()
		return nil, errors.New(resp.Status)
	}
	return rwc, nil
}

func (c *TunnelClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, client := range c.clients {
		client.CloseIdleConnections()
	}
	return nil
}

func (c *TunnelClient) ConnNum() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	var sum int
	for _, n := range c.connStat {
		sum += n
	}
	return sum
}

type rwc struct {
	respBody  io.ReadCloser
	reqBodyPW *io.PipeWriter
	onclose   func()
	once      sync.Once
}

func (r *rwc) Read(p []byte) (n int, err error) {
	return r.respBody.Read(p)
}

func (r *rwc) Write(p []byte) (n int, err error) {
	return r.reqBodyPW.Write(p)
}

func (r *rwc) Close() error {
	if r.onclose != nil {
		r.once.Do(r.onclose)
	}
	we := r.reqBodyPW.Close()
	re := r.respBody.Close()
	io.Copy(io.Discard, r.respBody)
	if we != nil {
		return we
	}
	return re
}

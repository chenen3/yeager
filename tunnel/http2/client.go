package http2

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"golang.org/x/net/http2"
)

type TunnelClient struct {
	addr    string
	tlsConf *tls.Config
	mu      sync.Mutex
	clients map[string]*http.Client
	stats   map[string]int // number of requests on http client
	idle    []*http.Client // clients not processing requests
	idleAt  []time.Time    //
}

func NewTunnelClient(addr string, tlsConf *tls.Config) *TunnelClient {
	return &TunnelClient{
		addr:    addr,
		tlsConf: tlsConf,
		clients: make(map[string]*http.Client),
		stats:   make(map[string]int),
	}
}

func (c *TunnelClient) putClient(key string, hc *http.Client) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stats[key]--
	if c.stats[key] == 0 {
		if cli, ok := c.clients[key]; ok {
			c.idle = append(c.idle, cli)
			c.idleAt = append(c.idleAt, time.Now())
		}
		delete(c.clients, key)
		delete(c.stats, key)
	}
}

// getConn tends to use existing client connections, create one if not exists.
//
// Since all requests are forwarded to the same tunnel server address,
// if a single global http2 client is used,
// it will cause too many streams on one connection,
// which is quite unfavorable when the network is unstable.
func (c *TunnelClient) getClient(key string) *http.Client {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stats[key]++

	client, ok := c.clients[key]
	if ok {
		return client
	}

	if len(c.idle) > 0 {
		i := -1
		for j := range c.idle {
			if time.Since(c.idleAt[j]) < idleTimeout {
				i = j
				break
			}
			// must close the connection before leaving,
			// otherwise it will run out of memory
			c.idle[j].CloseIdleConnections()
		}
		if i >= 0 {
			client = c.idle[i]
			// clients before index i, have been timeout
			c.idle = c.idle[i+1:]
			c.idleAt = c.idleAt[i+1:]
			c.clients[key] = client
			return client
		}
		c.idle = nil
		c.idleAt = nil
	}

	return c.addClientLocked(key)
}

func (c *TunnelClient) addClientLocked(key string) *http.Client {
	t := &http2.Transport{
		TLSClientConfig: c.tlsConf,
		DialTLSContext: func(ctx context.Context, network, addr string, cfg *tls.Config) (net.Conn, error) {
			d := &net.Dialer{
				Timeout:   5 * time.Second,
				KeepAlive: 30 * time.Second,
			}
			return tls.DialWithDialer(d, "tcp", addr, cfg)
		},
	}
	client := &http.Client{Transport: t}
	c.clients[key] = client
	return client
}

func (c *TunnelClient) DialContext(ctx context.Context, dst string) (io.ReadWriteCloser, error) {
	pr, pw := io.Pipe()
	req, err := http.NewRequest(http.MethodConnect, "https://"+c.addr, pr)
	if err != nil {
		return nil, err
	}
	req.Header.Add("dst", dst)
	req.Header.Set("User-Agent", "Chrome/76.0.3809.100")

	client := c.getClient(dst)
	// the client return Responses from servers once
	// the response headers have been received
	resp, err := client.Do(req)
	if err != nil {
		req.Body.Close()
		c.putClient(dst, client)
		return nil, errors.New("http2 request: " + err.Error())
	}
	if resp.StatusCode != http.StatusOK {
		req.Body.Close()
		resp.Body.Close()
		c.putClient(dst, client)
		return nil, errors.New(resp.Status)
	}

	rwc := &readWriteCloser{
		r: resp.Body,
		w: pw,
		onClose: func() {
			req.Body.Close()
			resp.Body.Close()
			c.putClient(dst, client)
		},
	}
	return rwc, nil
}

func (c *TunnelClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, client := range c.clients {
		client.CloseIdleConnections()
	}
	for _, client := range c.idle {
		client.CloseIdleConnections()
	}
	return nil
}

func (c *TunnelClient) ConnNum() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.clients) + len(c.idle)
}

type readWriteCloser struct {
	r       io.Reader
	w       io.Writer
	onClose func()
}

func (rwc *readWriteCloser) Read(p []byte) (n int, err error) {
	return rwc.r.Read(p)
}

func (rwc *readWriteCloser) Write(p []byte) (n int, err error) {
	return rwc.w.Write(p)
}

func (rwc *readWriteCloser) Close() error {
	if rwc.onClose != nil {
		rwc.onClose()
	}
	return nil
}

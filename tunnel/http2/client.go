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
	reqStat map[string]int // numbers of requests on specified dst
	idleCli []*http.Client
	idleAt  []time.Time // conresponding to idleClients
}

func NewTunnelClient(addr string, tlsConf *tls.Config) *TunnelClient {
	return &TunnelClient{
		addr:    addr,
		tlsConf: tlsConf,
		clients: make(map[string]*http.Client),
		reqStat: make(map[string]int),
	}
}

// h2Client returns a http2 client for dst, create one if not exists.
//
// Since all requests are forwarded to the same tunnel server address,
// if we only use one http2 client, according to the multiplexing feature,
// all requests will be transmitted on the same connection.
// When encountering the head-of-line blocking problem of TCP,
// all requests will be affected. Therefore using multiple clients
func (c *TunnelClient) h2Client(dst string) (client *http.Client, untrack func()) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.reqStat[dst]++
	untrack = func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		c.reqStat[dst]--
		if c.reqStat[dst] == 0 {
			delete(c.reqStat, dst)
			if cli, ok := c.clients[dst]; ok {
				c.idleCli = append(c.idleCli, cli)
				c.idleAt = append(c.idleAt, time.Now())
			}
			delete(c.clients, dst)
		}
	}

	client, ok := c.clients[dst]
	if ok {
		return client, untrack
	}

	if len(c.idleCli) > 0 {
		i := -1
		for j := range c.idleCli {
			if time.Since(c.idleAt[j]) < idleTimeout {
				i = j
				break
			}
			// must close the connection before leaving,
			// otherwise it will run out of memory
			c.idleCli[j].CloseIdleConnections()
		}
		if i >= 0 {
			client = c.idleCli[i]
			// clients before index i, have been timeout
			c.idleCli = c.idleCli[i+1:]
			c.idleAt = c.idleAt[i+1:]
			c.clients[dst] = client
			return client, untrack
		}
		c.idleCli = nil
		c.idleAt = nil
	}

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
	client = &http.Client{Transport: t}
	c.clients[dst] = client
	return client, untrack
}

func (c *TunnelClient) DialContext(ctx context.Context, dst string) (io.ReadWriteCloser, error) {
	pr, pw := io.Pipe()
	req, err := http.NewRequest(http.MethodConnect, "https://"+c.addr, pr)
	if err != nil {
		return nil, err
	}
	req.Header.Add("dst", dst)
	req.Header.Set("User-Agent", "Chrome/76.0.3809.100")

	client, untrack := c.h2Client(dst)
	// the client return Responses from servers once
	// the response headers have been received
	resp, err := client.Do(req)
	if err != nil {
		req.Body.Close()
		untrack()
		return nil, errors.New("http2 request: " + err.Error())
	}
	if resp.StatusCode != http.StatusOK {
		req.Body.Close()
		resp.Body.Close()
		untrack()
		return nil, errors.New(resp.Status)
	}

	rwc := &readWriteCloser{
		r: resp.Body,
		w: pw,
		onClose: func() {
			req.Body.Close()
			resp.Body.Close()
			untrack()
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
	for _, client := range c.idleCli {
		client.CloseIdleConnections()
	}
	return nil
}

func (c *TunnelClient) ConnNum() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.clients) + len(c.idleCli)
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

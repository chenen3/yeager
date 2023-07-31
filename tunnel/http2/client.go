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

	"github.com/chenen3/yeager/debug"
	"github.com/chenen3/yeager/obfs"
	"golang.org/x/net/http2"
)

type TunnelClient struct {
	addr    string
	tlsConf *tls.Config
	mu      sync.Mutex
	clients map[string]*http.Client
	reqStat map[string]int // numbers of requests on specified dst
	obfs    bool
}

func NewTunnelClient(addr string, tlsConf *tls.Config, obfs bool) *TunnelClient {
	return &TunnelClient{
		addr:    addr,
		tlsConf: tlsConf,
		obfs:    obfs,
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
			delete(c.clients, dst)
			debug.Printf("remove h2 client: %s", dst)
		}
	}

	client, ok := c.clients[dst]
	if ok {
		return client, untrack
	}

	t := &http2.Transport{
		DialTLSContext: func(ctx context.Context, network, addr string, _ *tls.Config) (net.Conn, error) {
			d := &net.Dialer{
				Timeout:   5 * time.Second,
				KeepAlive: 30 * time.Second,
			}
			return tls.DialWithDialer(d, "tcp", addr, c.tlsConf)
		},
	}
	client = &http.Client{Transport: t}
	c.clients[dst] = client
	return client, untrack
}

func (c *TunnelClient) DialContext(ctx context.Context, dst string) (io.ReadWriteCloser, error) {
	pr, pw := io.Pipe()
	req := &http.Request{
		Method:        "CONNECT",
		URL:           &url.URL{Scheme: "https", Host: c.addr},
		Header:        make(http.Header),
		Body:          pr,
		ContentLength: -1,
	}
	req.Header.Add("dst", dst)
	req.Header.Set("User-Agent", "Chrome/76.0.3809.100")

	client, untrack := c.h2Client(dst)
	// the client return Responses from servers once
	// the response headers have been received
	resp, err := client.Do(req)
	if err != nil {
		pw.Close()
		untrack()
		return nil, errors.New("http2 request: " + err.Error())
	}
	if resp.StatusCode != http.StatusOK {
		pw.Close()
		untrack()
		resp.Body.Close()
		return nil, errors.New(resp.Status)
	}

	rwc := &readWriteCloser{
		r: resp.Body,
		w: pw,
		onClose: func() {
			pw.Close()
			untrack()
			resp.Body.Close()
		},
	}
	if c.obfs {
		rwc.r = obfs.Reader(rwc.r)
		rwc.w = obfs.Writer(rwc.w)
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
	return len(c.clients)
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

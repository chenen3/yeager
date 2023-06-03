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

func (c *TunnelClient) client(dst string) (client *http.Client, closeFunc func()) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.connStat[dst]++
	closeFunc = func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		c.connStat[dst]--
		if c.connStat[dst] == 0 {
			delete(c.connStat, dst)
			delete(c.clients, dst)
			debug.Printf("remove h2 client: %s", dst)
		}
	}

	client, ok := c.clients[dst]
	if ok {
		return client, closeFunc
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
	c.clients[dst] = client
	return client, closeFunc
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

	client, closeClient := c.client(dst)
	resp, err := client.Do(req)
	if err != nil {
		pw.Close()
		closeClient()
		return nil, errors.New("http2 request: " + err.Error())
	}
	if resp.StatusCode != http.StatusOK {
		pw.Close()
		closeClient()
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		return nil, errors.New(resp.Status)
	}

	rwc := &rwc{
		rc:      resp.Body,
		wc:      pw,
		onclose: closeClient,
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

type rwc struct {
	rc      io.ReadCloser
	wc      io.WriteCloser
	once    sync.Once // guard onClose
	onclose func()
}

func (r *rwc) Read(p []byte) (n int, err error) {
	return r.rc.Read(p)
}

func (r *rwc) Write(p []byte) (n int, err error) {
	return r.wc.Write(p)
}

func (r *rwc) Close() error {
	if r.onclose != nil {
		r.once.Do(r.onclose)
	}
	we := r.wc.Close()
	// drain the response body
	io.Copy(io.Discard, r.rc)
	re := r.rc.Close()
	if we != nil {
		return we
	}
	return re
}

package wss

import (
	"context"
	"crypto/tls"
	"errors"
	"io"

	"github.com/chenen3/yeager/tunnel"
	"golang.org/x/net/websocket"
)

type TunnelClient struct {
	target  string
	tlsConf *tls.Config
}

func NewTunnelClient(target string, tlsConfig *tls.Config) *TunnelClient {
	return &TunnelClient{
		target:  target,
		tlsConf: tlsConfig,
	}
}

func (c *TunnelClient) DialContext(ctx context.Context, dst string) (io.ReadWriteCloser, error) {
	d := tls.Dialer{Config: c.tlsConf}
	tlsconn, err := d.DialContext(ctx, "tcp", c.target)
	if err != nil {
		return nil, errors.New("tls dial: " + err.Error())
	}

	config, err := websocket.NewConfig("ws://"+c.target+"/relay", "http://localhost")
	if err != nil {
		return nil, err
	}
	wsconn, err := websocket.NewClient(config, tlsconn)
	if err != nil {
		return nil, errors.New("create websocket client: " + err.Error())
	}

	if err := tunnel.WriteHeader(wsconn, dst); err != nil {
		return nil, errors.New("write header: " + err.Error())
	}
	return wsconn, nil
}

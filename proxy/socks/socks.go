// Package socks provides a SOCKS version 5 server implementation.
package socks

import (
	"encoding/json"
	"yeager/proxy"
)

func init() {
	proxy.RegisterInboundBuilder(protocol, func(setting json.RawMessage) (proxy.Inbound, error) {
		conf := new(Config)
		if err := json.Unmarshal(setting, conf); err != nil {
			return nil, err
		}
		return NewServer(conf), nil
	})
}

const protocol = "socks"

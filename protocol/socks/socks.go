// Package socks provides a SOCKS version 5 server implementation.
package socks

import (
	"encoding/json"
	"yeager/protocol"
)

func init() {
	protocol.RegisterInboundBuilder("socks", func(setting json.RawMessage) (protocol.Inbound, error) {
		conf := new(Config)
		if err := json.Unmarshal(setting, conf); err != nil {
			return nil, err
		}
		return NewServer(conf), nil
	})
}

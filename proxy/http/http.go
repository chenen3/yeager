package http

import (
	"encoding/json"
	"yeager/proxy"
)

func init() {
	proxy.RegisterInboundBuilder(Tag, func(setting json.RawMessage) (inbound proxy.Inbound, err error) {
		conf := new(Config)
		if err = json.Unmarshal(setting, conf); err != nil {
			return nil, err
		}
		return NewServer(conf), nil
	})
}

const Tag = "http"
package yeager

import (
	"encoding/json"
	"yeager/proxy"
)

func init() {
	proxy.RegisterOutboundBuilder("yeager", func(setting json.RawMessage) (proxy.Outbound, error) {
		conf := new(ClientConfig)
		if err := json.Unmarshal(setting, conf); err != nil {
			return nil, err
		}
		return NewClient(conf), nil
	})

	proxy.RegisterInboundBuilder("yeager", func(setting json.RawMessage) (proxy.Inbound, error) {
		conf := new(ServerConfig)
		if err := json.Unmarshal(setting, conf); err != nil {
			return nil, err
		}
		return NewServer(conf), nil
	})
}

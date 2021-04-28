package yeager

import (
	"encoding/json"
	"yeager/protocol"
)

func init() {
	protocol.RegisterOutboundBuilder("yeager", func(setting json.RawMessage) (protocol.Outbound, error) {
		conf := new(ClientConfig)
		if err := json.Unmarshal(setting, conf); err != nil {
			return nil, err
		}
		return NewClient(conf), nil
	})

	protocol.RegisterInboundBuilder("yeager", func(setting json.RawMessage) (protocol.Inbound, error) {
		conf := new(ServerConfig)
		if err := json.Unmarshal(setting, conf); err != nil {
			return nil, err
		}
		return NewServer(conf), nil
	})
}

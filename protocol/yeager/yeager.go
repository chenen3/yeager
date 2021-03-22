package yeager

import (
	"encoding/json"
	"yeager/protocol"
)

func init() {
	protocol.RegisterOutboundBuilder("yeager", func(setting json.RawMessage) (protocol.Outbound, error) {
		bs, err := setting.MarshalJSON()
		if err != nil {
			return nil, err
		}
		conf := new(ClientConfig)
		err = json.Unmarshal(bs, conf)
		if err != nil {
			return nil, err
		}
		return NewClient(conf), nil
	})

	protocol.RegisterInboundBuilder("yeager", func(setting json.RawMessage) (protocol.Inbound, error) {
		bs, err := setting.MarshalJSON()
		if err != nil {
			return nil, err
		}
		conf := new(ServerConfig)
		err = json.Unmarshal(bs, conf)
		if err != nil {
			return nil, err
		}
		return NewServer(conf)
	})
}

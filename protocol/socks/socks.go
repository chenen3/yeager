package socks

import (
	"encoding/json"
	"yeager/protocol"
)

func init() {
	protocol.RegisterInboundBuilder("socks", func(setting json.RawMessage) (protocol.Inbound, error) {
		data, err := setting.MarshalJSON()
		if err != nil {
			return nil, err
		}
		conf := new(Config)
		err = json.Unmarshal(data, conf)
		if err != nil {
			return nil, err
		}
		return NewServer(conf)
	})
}

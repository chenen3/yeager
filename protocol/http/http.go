package http

import (
	"encoding/json"
	"yeager/protocol"
)

func init() {
	protocol.RegisterInboundBuilder("http", func(setting json.RawMessage) (inbound protocol.Inbound, err error) {
		conf := new(Config)
		err = json.Unmarshal(setting, conf)
		if err != nil {
			return nil, err
		}
		return NewServer(conf), nil
	})
}

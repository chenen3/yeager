package yeager

import (
	"encoding/json"
	"os"
	"yeager/proxy"
)

func init() {
	proxy.RegisterOutboundBuilder(Tag, func(setting json.RawMessage) (proxy.Outbound, error) {
		var conf ClientConfig
		err := json.Unmarshal(setting, &conf)
		if err != nil {
			return nil, err
		}
		return NewClient(&conf), nil
	})

	proxy.RegisterInboundBuilder(Tag, func(setting json.RawMessage) (proxy.Inbound, error) {
		var conf ServerConfig
		var err error
		err = json.Unmarshal(setting, &conf)
		if err != nil {
			return nil, err
		}

		conf.certPEMBlock, err = os.ReadFile(conf.CertFile)
		if err != nil {
			return nil, err
		}
		conf.keyPEMBlock, err = os.ReadFile(conf.KeyFile)
		if err != nil {
			return nil, err
		}

		return NewServer(&conf), nil
	})
}

const Tag = "yeager"

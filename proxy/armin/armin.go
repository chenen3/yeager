package armin

import (
	"encoding/json"
	"errors"
	"os"
	"yeager/proxy"
)

const protocol = "armin"

func init() {
	proxy.RegisterOutboundBuilder(protocol, func(setting json.RawMessage) (proxy.Outbound, error) {
		var conf ClientConfig
		err := json.Unmarshal(setting, &conf)
		if err != nil {
			return nil, err
		}
		if conf.TLS.ServerName == "" {
			conf.TLS.ServerName = conf.Host
		}
		return NewClient(&conf)
	})

	proxy.RegisterInboundBuilder(protocol, func(setting json.RawMessage) (proxy.Inbound, error) {
		var conf ServerConfig
		var err error
		err = json.Unmarshal(setting, &conf)
		if err != nil {
			return nil, err
		}

		conf.TLS.certPEMBlock, err = os.ReadFile(conf.TLS.CertFile)
		if err != nil {
			return nil, errors.New("read tls certificate file err: " + err.Error())
		}
		conf.TLS.keyPEMBlock, err = os.ReadFile(conf.TLS.KeyFile)
		if err != nil {
			return nil, errors.New("read tls key file err: " + err.Error())
		}

		return NewServer(&conf)
	})
}

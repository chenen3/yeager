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

		if conf.Security == "tls" {
			bs, err := os.ReadFile(conf.TLS.CertificateFile)
			if err != nil {
				return nil, errors.New("read tls certificate file err: " + err.Error())
			}
			conf.TLS.certPEMBlock = bs

			keyBS, keyErr := os.ReadFile(conf.TLS.KeyFile)
			if keyErr != nil {
				return nil, errors.New("read tls key file err: " + keyErr.Error())
			}
			conf.TLS.keyPEMBlock = keyBS
		}

		return NewServer(&conf)
	})
}

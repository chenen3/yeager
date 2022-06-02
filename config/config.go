package config

import (
	"encoding/json"

	"gopkg.in/yaml.v3"
)

// Load read config from bytes
func Load(bs []byte) (Config, error) {
	var yc Config
	err := yaml.Unmarshal(bs, &yc)
	if err == nil {
		return yc, nil
	}

	var jc Config
	err = json.Unmarshal(bs, &jc)
	return jc, err
}

type Config struct {
	// 入站代理
	Inbounds Inbounds `json:"inbounds,omitempty" yaml:"inbounds,omitempty"`
	// 出站代理
	Outbounds []YeagerClient `json:"outbounds,omitempty" yaml:"outbounds,omitempty"`
	// 路由规则
	Rules []string `json:"rules,omitempty" yaml:"rules,omitempty"`
	// verbose logging
	Verbose bool `json:"verbose,omitempty" yaml:"verbose,omitempty"`
	// expose runtime metrics for debugging and profiling, developers only
	Debug bool `json:"debug,omitempty" yaml:"debug,omitempty"`
}

type Inbounds struct {
	SOCKS  *SOCKS        `json:"socks,omitempty" yaml:"socks,omitempty"`
	HTTP   *HTTP         `json:"http,omitempty" yaml:"http,omitempty"`
	Yeager *YeagerServer `json:"yeager,omitempty" yaml:"yeager,omitempty"`
}

type SOCKS struct {
	Listen string `json:"listen" yaml:"listen"`
}

type HTTP struct {
	Listen string `json:"listen" yaml:"listen"`
}

type Transport string

const (
	TransTCP  Transport = "tcp" // plain text
	TransGRPC Transport = "grpc"
	TransQUIC Transport = "quic"

	// deprecated. Infrequently used
	// TransTLS Transport = "tls"
)

type MutualTLS struct {
	CertFile string `json:"certFile" yaml:"certFile"`
	CertPEM  string `json:"certPEM,omitempty" yaml:"certPEM,omitempty"`
	KeyFile  string `json:"keyFile" yaml:"keyFile"`
	KeyPEM   string `json:"keyPEM,omitempty" yaml:"keyPEM,omitempty"`
	CAFile   string `json:"caFile" yaml:"caFile"`
	CAPEM    string `json:"caPEM,omitempty" yaml:"caPEM,omitempty"`
}

type YeagerServer struct {
	Listen    string    `json:"listen" yaml:"listen"`
	Transport Transport `json:"transport" yaml:"transport"`
	// unavailable when transport is tcp
	MutualTLS MutualTLS `json:"mtls,omitempty" yaml:"mtls,omitempty"`
}

type YeagerClient struct {
	Tag string `json:"tag" yaml:"tag"`
	// server address to be connected
	Address   string    `json:"address" yaml:"address"`
	Transport Transport `json:"transport" yaml:"transport"`
	// unavailable when transport is tcp
	MutualTLS MutualTLS `json:"mtls,omitempty" yaml:"mtls,omitempty"`

	// optional
	ConnectionPoolSize int `json:"connectionPoolSize,omitempty" yaml:"connectionPoolSize,omitempty"`
}

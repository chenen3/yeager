package config

import "gopkg.in/yaml.v3"

// Load read config from bytes
func Load(bs []byte) (Config, error) {
	var c Config
	err := yaml.Unmarshal(bs, &c)
	return c, err
}

type Config struct {
	// socks 代理监听地址
	SOCKSListen string `yaml:"socksListen,omitempty"`
	// http 代理监听地址
	HTTPListen string `yaml:"httpListen,omitempty"`
	// yeager入站代理
	Inbounds []YeagerServer `yaml:"inbounds,omitempty"`
	// yeager出站代理
	Outbounds []YeagerClient `yaml:"outbounds,omitempty"`
	// 路由规则
	Rules []string `yaml:"rules,omitempty"`
	// verbose logging
	Verbose bool `yaml:"verbose,omitempty"`
	// expose runtime metrics for debugging and profiling, developers only
	Debug bool `yaml:"debug,omitempty"`
}

type Transport string

const (
	TransTCP  Transport = "tcp" // plain text
	TransGRPC Transport = "grpc"
	TransQUIC Transport = "quic"

	// deprecated. Infrequently used
	// TransTLS Transport = "tls"
)

type TLS struct {
	CertFile string `yaml:"certFile,omitempty"`
	CertPEM  string `yaml:"certPEM,omitempty"`
	KeyFile  string `yaml:"keyFile,omitempty"`
	KeyPEM   string `yaml:"keyPEM,omitempty"`
	CAFile   string `yaml:"caFile,omitempty"`
	CAPEM    string `yaml:"caPEM,omitempty"`
}

type YeagerServer struct {
	Listen    string    `yaml:"listen"`
	Transport Transport `yaml:"transport"`
	// unavailable when transport is tcp
	TLS TLS `yaml:"tls,omitempty"`
}

type YeagerClient struct {
	Tag string `yaml:"tag"`
	// server address to be connected
	Address   string    `yaml:"address"`
	Transport Transport `yaml:"transport"`
	// unavailable when transport is tcp
	TLS TLS `yaml:"tls,omitempty"`

	// optional
	ConnectionPoolSize int `yaml:"connectionPoolSize,omitempty"`
}

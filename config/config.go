package config

import (
	"encoding/json"
	"io"
)

var c Config

func C() Config {
	return c
}

// Load read config from reader, and update the global config instance
func Load(r io.Reader) (Config, error) {
	err := json.NewDecoder(r).Decode(&c)
	if err != nil {
		return Config{}, err
	}
	return c, nil
}

type Config struct {
	Inbounds  Inbounds        `json:"inbounds,omitempty"`  // 入站代理
	Outbounds []*YeagerClient `json:"outbounds,omitempty"` // 出站代理
	Rules     []string        `json:"rules,omitempty"`     // 路由规则
	// verbose logging
	Verbose bool `json:"verbose,omitempty"`
	// 如何预估连接池大小，参考 proxy/yeager/transport/grpc/pool.go
	ConnectionPoolSize int `json:"connectionPoolSize,omitempty"`
	// expose runtime metrics for debugging and profiling, developers only
	Debug bool `json:"debug,omitempty"`
}

type Inbounds struct {
	SOCKS  *SOCKS        `json:"socks,omitempty"`
	HTTP   *HTTP         `json:"http,omitempty"`
	Yeager *YeagerServer `json:"yeager,omitempty"`
}

type SOCKS struct {
	Listen string `json:"listen"`
}

type HTTP struct {
	Listen string `json:"listen"`
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
	CertFile string `json:"certFile"`
	CertPEM  []byte `json:"-"`
	KeyFile  string `json:"keyFile"`
	KeyPEM   []byte `json:"-"`
	CAFile   string `json:"caFile"`
	CAPEM    []byte `json:"-"`
}

type YeagerServer struct {
	Listen    string    `json:"listen"`
	Transport Transport `json:"transport"`
	MutualTLS MutualTLS `json:"mtls,omitempty"` // unavailable when transport is tcp
}

type YeagerClient struct {
	Tag       string    `json:"tag"`     // 出站标记，用于路由规则指定出站代理
	Address   string    `json:"address"` // server address to be connected
	Transport Transport `json:"transport"`
	MutualTLS MutualTLS `json:"mtls,omitempty"` // unavailable when transport is tcp
}

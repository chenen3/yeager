package config

import (
	"encoding/json"
	"errors"
	"os"
)

var c Config

func C() Config {
	return c
}

// LoadJSON load config from bytes in JSON format
func LoadJSON(bs []byte) (*Config, error) {
	var conf Config
	err := json.Unmarshal(bs, &conf)
	if err != nil {
		return nil, err
	}

	c = conf
	return &conf, nil
}

// LoadFile load config from JSON file
func LoadFile(filename string) (*Config, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}

	var c Config
	err = json.NewDecoder(f).Decode(&c)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

type Config struct {
	Inbounds  Inbounds        `json:"inbounds,omitempty"`  // 入站代理
	Outbounds []*YeagerClient `json:"outbounds,omitempty"` // 出站代理
	Rules     []string        `json:"rules,omitempty"`     // 路由规则
	Verbose   bool            `json:"verbose,omitempty"`   // verbose log

	// developer only
	Debug bool `json:"debug,omitempty"`
	// 如何预估连接池大小，参考 proxy/yeager/transport/grpc/pool.go
	GrpcChannelPoolSize int `json:"grpcChannelPoolSize,omitempty"`
}

type Inbounds struct {
	SOCKS *struct {
		Listen string `json:"listen"`
	} `json:"socks,omitempty"`

	HTTP *struct {
		Listen string `json:"listen"`
	} `json:"http,omitempty"`

	Yeager *YeagerServer `json:"yeager,omitempty"`
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

const (
	EnvYeagerAddress   = "YEAGER_ADDRESS"
	EnvYeagerTransport = "YEAGER_TRANSPORT"
)

// Deprecated
// LoadEnv generate configuration from environment variables,
// suitable for server side plaintext
func LoadEnv() (conf *Config, err error) {
	address, ok := os.LookupEnv(EnvYeagerAddress)
	if !ok {
		return nil, errors.New("missing " + EnvYeagerAddress)
	}
	transport, ok := os.LookupEnv(EnvYeagerTransport)
	if !ok {
		return nil, errors.New("missing " + EnvYeagerTransport)
	}

	sc := &YeagerServer{
		Listen:    address,
		Transport: Transport(transport),
	}
	return &Config{Inbounds: Inbounds{Yeager: sc}}, nil
}

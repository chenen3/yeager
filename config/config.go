package config

import (
	"encoding/json"
	"os"
	"strings"
)

type Config struct {
	Inbounds  Inbounds        `json:"inbounds,omitempty"`  // 入站代理
	Outbounds []*YeagerClient `json:"outbounds,omitempty"` // 出站代理
	Rules     []string        `json:"rules,omitempty"`     // 路由规则
}

type Inbounds struct {
	SOCKS  *SOCKSProxy   `json:"socks,omitempty"`
	HTTP   *HTTPProxy    `json:"http,omitempty"`
	Yeager *YeagerServer `json:"yeager,omitempty"`
}

type SOCKSProxy struct {
	Address string `json:"address"`
}

type HTTPProxy struct {
	Address string `json:"address"`
}

type YeagerServer struct {
	Address   string `json:"address"`
	UUID      string `json:"uuid"`
	Transport string `json:"transport"` // tcp, tls, grpc
	// if transport field is grpc and plaintext field is true,
	// the server would accept grpc request in plaintext, and
	// ignores certificate config. please do not use plaintext
	// unless you know what you are doing
	Plaintext bool `json:"plaintext,omitempty"`

	// automated manage certificate
	ACME *ACME `json:"acme,omitempty"`

	// manually manage certificate
	CertFile     string `json:"certFile,omitempty"`
	KeyFile      string `json:"keyFile,omitempty"`
	CertPEMBlock []byte `json:"-"`
	KeyPEMBlock  []byte `json:"-"`
}

type ACME struct {
	Domain    string `json:"domain,omitempty"`
	Email     string `json:"email,omitempty"`
	StagingCA bool   `json:"stagingCA,omitempty"` // use staging CA in testing, in case lock out
}

type YeagerClient struct {
	Tag       string `json:"tag"` // 出站标记，用于路由规则指定出站代理
	Address   string `json:"address"`
	UUID      string `json:"uuid"`
	Transport string `json:"transport"` // tls, grpc
	// if transport field is grpc and plaintext field is true,
	// the client would send grpc request in plaintext, please
	// do not use plaintext unless you know what you are doing
	Plaintext bool `json:"plaintext,omitempty"`
	Insecure  bool `json:"insecure,omitempty"` // allow insecure TLS
}

func LoadFile(filename string) (*Config, error) {
	bs, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	return LoadBytes(bs)
}

func LoadBytes(bs []byte) (*Config, error) {
	conf := new(Config)
	err := json.Unmarshal(bs, conf)
	return conf, err
}

// LoadEnv generate configuration from environment variables, only support server-side plaintext traffic
func LoadEnv() *Config {
	address := os.Getenv("YEAGER_ADDRESS")
	uuid := os.Getenv("YEAGER_UUID")
	transport := os.Getenv("YEAGER_TRANSPORT")
	if address == "" || uuid == "" || transport == "" {
		return nil
	}

	domain := os.Getenv("YEAGER_DOMAIN")
	email := os.Getenv("YEAGER_EMAIL")
	var stagingCA bool
	if strings.EqualFold(os.Getenv("YEAGER_STAGINGCA"), "true") {
		stagingCA = true
	}
	var plaintext bool
	if strings.EqualFold(os.Getenv("YEAGER_PLAINTEXT"), "true") {
		plaintext = true
	}

	ac := &YeagerServer{
		Address:   address,
		UUID:      uuid,
		Transport: transport,
		Plaintext: plaintext,
		ACME: &ACME{
			Domain:    domain,
			Email:     email,
			StagingCA: stagingCA,
		},
	}
	return &Config{Inbounds: Inbounds{Yeager: ac}}
}

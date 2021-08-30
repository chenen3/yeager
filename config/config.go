package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
)

type Config struct {
	Inbounds  Inbounds             `json:"inbounds,omitempty"`  // 入站代理
	Outbounds []*ArminClientConfig `json:"outbounds,omitempty"` // 出站代理
	Rules     []string             `json:"rules,omitempty"`     // 路由规则
}

type Inbounds struct {
	SOCKS *SOCKSServerConfig `json:"socks,omitempty"`
	HTTP  *HTTPServerConfig  `json:"http,omitempty"`
	Armin *ArminServerConfig `json:"armin,omitempty"`
}

type SOCKSServerConfig struct {
	Address string `json:"address"`
}

type HTTPServerConfig struct {
	Address string `json:"address"`
}

type ArminServerConfig struct {
	Address   string `json:"address"`
	UUID      string `json:"uuid"`
	Transport string `json:"transport"` // tcp, tls, grpc
	// if plaintext is true and transport is grpc,
	// the server would accept grpc request in plaintext,
	// certFile and keyFile will be ignored.
	Plaintext bool   `json:"plaintext,omitempty"`
	CertFile  string `json:"certFile,omitempty"`
	KeyFile   string `json:"keyFile,omitempty"`

	CertPEMBlock []byte `json:"-"`
	KeyPEMBlock  []byte `json:"-"`
}

type ArminClientConfig struct {
	Tag       string `json:"tag"` // 出站标记，用于路由规则指定出站代理
	Address   string `json:"address"`
	UUID      string `json:"uuid"`
	Transport string `json:"transport"` // tls, grpc
	// if plaintext is true and transport is grpc,
	// the client would send grpc request in plaintext
	Plaintext  bool   `json:"plaintext,omitempty"`
	ServerName string `json:"serverName,omitempty"`
	Insecure   bool   `json:"insecure,omitempty"` // allow insecure TLS
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
	if err := json.Unmarshal(bs, conf); err != nil {
		return nil, err
	}

	if ac := conf.Inbounds.Armin; ac != nil && !ac.Plaintext {
		bs, err := os.ReadFile(ac.CertFile)
		if err != nil {
			return nil, errors.New("read tls certificate file err: " + err.Error())
		}
		ac.CertPEMBlock = bs

		keyBS, keyErr := os.ReadFile(ac.KeyFile)
		if keyErr != nil {
			return nil, errors.New("read tls key file err: " + keyErr.Error())
		}
		ac.KeyPEMBlock = keyBS
	}

	for _, ac := range conf.Outbounds {
		if ac.ServerName == "" {
			host, _, err := net.SplitHostPort(ac.Address)
			if err != nil {
				return nil, fmt.Errorf("failed to parse address: %s, err: %s", ac.Address, err)
			}
			ac.ServerName = host
		}
	}

	return conf, nil
}

// LoadEnv generate configuration from environment variables, only support plaintext traffic
func LoadEnv() *Config {
	address := os.Getenv("ARMIN_ADDRESS")
	uuid := os.Getenv("ARMIN_UUID")
	transport := os.Getenv("ARMIN_TRANSPORT")
	if address == "" || uuid == "" || transport == "" {
		return nil
	}

	ac := &ArminServerConfig{
		Address:   address,
		UUID:      uuid,
		Transport: transport,
	}
	plaintext := os.Getenv("ARMIN_PLAINTEXT")
	if strings.EqualFold(plaintext, "true") {
		ac.Plaintext = true
	}

	return &Config{Inbounds: Inbounds{Armin: ac}}
}

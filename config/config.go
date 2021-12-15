package config

import (
	"encoding/json"
	"errors"
	"fmt"
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
	bs, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	return LoadJSON(bs)
}

type Config struct {
	Inbounds  Inbounds        `json:"inbounds,omitempty"`  // 入站代理
	Outbounds []*YeagerClient `json:"outbounds,omitempty"` // 出站代理
	Rules     []string        `json:"rules,omitempty"`     // 路由规则

	// developer only
	Debug bool `json:"debug,omitempty"`

	// 参考 transport/grpc/pool.go 如何预估连接池大小
	GrpcChannelPoolSize int `json:"grpcChannelPoolSize,omitempty"`
}

type Inbounds struct {
	SOCKS  *SOCKSProxy   `json:"socks,omitempty"`
	HTTP   *HTTPProxy    `json:"http,omitempty"`
	Yeager *YeagerServer `json:"yeager,omitempty"`
}

type SOCKSProxy struct {
	Listen string `json:"listen"`
}

type HTTPProxy struct {
	Listen string `json:"listen"`
}

type Transport string

const (
	TransTCP  Transport = "tcp"
	TransGRPC Transport = "grpc"
	TransQUIC Transport = "quic"
)

type ServerSecurityType string

const (
	NoSecurity ServerSecurityType = ""           // no security, means plaintext
	TLS        ServerSecurityType = "tls"        // security by TLS, manage certificate manually
	TLSAcme    ServerSecurityType = "tls-acme"   // security by TLS, manage certificate automatically
	TLSMutual  ServerSecurityType = "tls-mutual" // security by mutual TLS
)

type Tls struct {
	CertFile string `json:"certFile"`
	CertPEM  []byte `json:"-"`
	KeyFile  string `json:"keyFile"`
	KeyPEM   []byte `json:"-"`
}

type Acme struct {
	Domain string `json:"domain"`
}

type Mtls struct {
	CertFile     string `json:"certFile"`
	CertPEM      []byte `json:"-"`
	KeyFile      string `json:"keyFile"`
	KeyPEM       []byte `json:"-"`
	ClientCAFile string `json:"clientCAFile"`
	ClientCA     []byte `json:"-"`
}

type YeagerServer struct {
	Listen    string             `json:"listen"`
	UUID      string             `json:"uuid"` // ignored when Security is TLSMutual
	Transport Transport          `json:"transport"`
	Security  ServerSecurityType `json:"security"`
	TLS       Tls                `json:"tls,omitempty"`  // available when Security is TLS
	ACME      Acme               `json:"acme,omitempty"` // available when Security is TLSAcme
	MTLS      Mtls               `json:"mtls,omitempty"` // available when Security is TLSMutual
}

type ClientSecurityType string

const (
	ClientNoSecurity ClientSecurityType = ""           // no security, means plaintext
	ClientTLS        ClientSecurityType = "tls"        // security by TLS, manage certificate manually
	ClientTLSMutual  ClientSecurityType = "tls-mutual" // security by mutual TLS
)

type ClientTls struct {
	Insecure bool `json:"insecure,omitempty"` // allow insecure
}

type ClientMTLS struct {
	CertFile   string `json:"certFile"`
	CertPEM    []byte `json:"-"`
	KeyFile    string `json:"keyFile"`
	KeyPEM     []byte `json:"-"`
	RootCAFile string `json:"rootCAFile"`
	RootCA     []byte `json:"-"`
}

type YeagerClient struct {
	Tag       string             `json:"tag"` // 出站标记，用于路由规则指定出站代理
	Address   string             `json:"address"`
	UUID      string             `json:"uuid"` // ignored when Security is TLSMutual
	Transport Transport          `json:"transport"`
	Security  ClientSecurityType `json:"security"`
	TLS       ClientTls          `json:"tls,omitempty"`  // available when Security is ClientTLS
	MTLS      ClientMTLS         `json:"mtls,omitempty"` // available when Security is ClientTLSMutual
}

const (
	EnvYeagerAddress   = "YEAGER_ADDRESS"
	EnvYeagerUUID      = "YEAGER_UUID"
	EnvYeagerTransport = "YEAGER_TRANSPORT"
	EnvYeagerDomain    = "YEAGER_DOMAIN"
	EnvYeagerSecurity  = "YEAGER_SECURITY"
)

// Deprecated
// LoadEnv generate configuration from environment variables,
// suitable for server side plaintext or TLS with ACME
func LoadEnv() (conf *Config, err error, foundEnv bool) {
	address, foundAddr := os.LookupEnv(EnvYeagerAddress)
	uuid, foundUUID := os.LookupEnv(EnvYeagerUUID)
	transport, foundTransport := os.LookupEnv(EnvYeagerTransport)
	domain, foundDomain := os.LookupEnv(EnvYeagerDomain)
	if foundAddr || foundUUID || foundTransport || foundDomain == false {
		return nil, nil, false
	}

	foundEnv = true
	if address == "" {
		return nil, fmt.Errorf("required env %s", EnvYeagerAddress), foundEnv
	}
	if uuid == "" {
		return nil, fmt.Errorf("required env %s", EnvYeagerUUID), foundEnv
	}
	if transport == "" {
		return nil, fmt.Errorf("required env %s", EnvYeagerTransport), foundEnv
	}
	if domain == "" {
		return nil, fmt.Errorf("required env %s", EnvYeagerDomain), foundEnv
	}

	security := os.Getenv(EnvYeagerSecurity)

	sc := &YeagerServer{
		Listen:    address,
		UUID:      uuid,
		Transport: Transport(transport),
		Security:  ServerSecurityType(security),
	}
	switch sc.Security {
	case NoSecurity:
	case TLSAcme:
		sc.ACME = Acme{Domain: domain}
	default:
		return nil, errors.New("LoadEnv does not support security: " + security), foundEnv
	}

	return &Config{Inbounds: Inbounds{Yeager: sc}}, nil, foundEnv
}

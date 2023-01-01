package config

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/chenen3/yeager/cert"
)

// Load loads configuration from the given bytes
func Load(bs []byte) (Config, error) {
	var c Config
	err := json.Unmarshal(bs, &c)
	return c, err
}

type Config struct {
	SOCKSListen   string         `json:"socksListen,omitempty"`
	HTTPListen    string         `json:"httpListen,omitempty"`
	TunnelListens []TunnelListen `json:"tunnelListens,omitempty"`
	TunnelClients []TunnelClient `json:"tunnelClients,omitempty"`
	Rules         []string       `json:"rules,omitempty"`
	// developer only: enable debug logging, start HTTP server for profiling
	Debug bool `json:"debug,omitempty"`
}

type TunnelType string

const (
	TunGRPC TunnelType = "grpc"
	TunQUIC TunnelType = "quic"
)

type Lines []string

// Merge merges list of strings into multiple lines
func (l Lines) Merge() string {
	return strings.Join(l, "\n")
}

type TunnelListen struct {
	Type     TunnelType `json:"type"`
	Listen   string     `json:"listen"`
	CertFile string     `json:"certFile,omitempty"`
	CertPEM  Lines      `json:"certPEM,omitempty"`
	KeyFile  string     `json:"keyFile,omitempty"`
	KeyPEM   Lines      `json:"keyPEM,omitempty"`
	CAFile   string     `json:"caFile,omitempty"`
	CAPEM    Lines      `json:"caPEM,omitempty"`
}

type TunnelClient struct {
	Type    TunnelType `json:"type"`
	Policy  string     `json:"policy"`
	Address string     `json:"address"` // target server address

	CertFile string `json:"certFile,omitempty"`
	CertPEM  Lines  `json:"certPEM,omitempty"`
	KeyFile  string `json:"keyFile,omitempty"`
	KeyPEM   Lines  `json:"keyPEM,omitempty"`
	CAFile   string `json:"caFile,omitempty"`
	CAPEM    Lines  `json:"caPEM,omitempty"`

	PoolSize int `json:"poolSize,omitempty"`
}

func allocPort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	ln.Close()
	return ln.Addr().(*net.TCPAddr).Port, nil
}

// Pair make a pair of client and server config
func Pair(host string) (srv, cli Config, err error) {
	cert, err := cert.Generate(host)
	if err != nil {
		return srv, cli, err
	}
	tunnelPort, err := allocPort()
	if err != nil {
		return srv, cli, err
	}

	srv = Config{
		TunnelListens: []TunnelListen{
			{
				Listen:  fmt.Sprintf("0.0.0.0:%d", tunnelPort),
				Type:    TunGRPC,
				CAPEM:   strings.Split(string(cert.RootCert), "\n"),
				CertPEM: strings.Split(string(cert.ServerCert), "\n"),
				KeyPEM:  strings.Split(string(cert.ServerKey), "\n"),
			},
		},
	}

	socksProxyPort, err := allocPort()
	if err != nil {
		return srv, cli, err
	}
	httpProxyPort, err := allocPort()
	if err != nil {
		return srv, cli, err
	}
	cli = Config{
		SOCKSListen: fmt.Sprintf("127.0.0.1:%d", socksProxyPort),
		HTTPListen:  fmt.Sprintf("127.0.0.1:%d", httpProxyPort),
		TunnelClients: []TunnelClient{
			{
				Policy:  "proxy",
				Address: fmt.Sprintf("%s:%d", host, tunnelPort),
				Type:    TunGRPC,
				CAPEM:   strings.Split(string(cert.RootCert), "\n"),
				CertPEM: strings.Split(string(cert.ClientCert), "\n"),
				KeyPEM:  strings.Split(string(cert.ClientKey), "\n"),
			},
		},
		Rules: []string{"final,proxy"},
	}
	return srv, cli, nil
}

// Generate generates the corrensponding client and server config file
func Generate(ip, srvConfOutput, cliConfOutput string) error {
	_, err := os.Stat(srvConfOutput)
	if err == nil {
		return fmt.Errorf("found %s, operation aborted", srvConfOutput)
	}
	_, err = os.Stat(cliConfOutput)
	if err == nil {
		return fmt.Errorf("found %s, operation aborted", cliConfOutput)
	}

	srvConf, cliConf, err := Pair(ip)
	if err != nil {
		return fmt.Errorf("failed to generate config: %s", err)
	}
	if len(srvConf.TunnelListens) == 0 {
		return fmt.Errorf("no tunnelListens in server config")
	}
	srvConf.TunnelListens[0].Listen = "0.0.0.0:9001"
	bs, err := json.Marshal(srvConf)
	if err != nil {
		return fmt.Errorf("failed to marshal server config: %s", err)
	}
	err = os.WriteFile(srvConfOutput, bs, 0644)
	if err != nil {
		return fmt.Errorf("failed to write server config: %s", err)
	}
	fmt.Printf("generated server config file: %s\n", srvConfOutput)

	if len(cliConf.TunnelClients) == 0 {
		return fmt.Errorf("no tunnelClients in client config")
	}
	cliConf.TunnelClients[0].Address = fmt.Sprintf("%s:%d", ip, 9001)
	cliConf.SOCKSListen = "127.0.0.1:1080"
	cliConf.HTTPListen = "127.0.0.1:8080"
	cliConf.Rules = []string{
		"ip-cidr,127.0.0.1/8,direct",
		"ip-cidr,192.168.0.0/16,direct",
		"ip-cidr,172.16.0.0/12,direct",
		"ip-cidr,10.0.0.0/8,direct",
		"domain,localhost,direct",
		"geosite,cn,direct",
		"geosite,apple@cn,direct",
		"final,proxy",
	}
	bs, err = json.Marshal(cliConf)
	if err != nil {
		return fmt.Errorf("failed to marshal client config: %s", err)
	}
	err = os.WriteFile(cliConfOutput, bs, 0644)
	if err != nil {
		return fmt.Errorf("failed to write client config: %s", err)
	}
	fmt.Printf("generated client config file: %s\n", cliConfOutput)
	return nil
}

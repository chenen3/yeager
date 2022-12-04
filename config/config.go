package config

import (
	"fmt"
	"net"
	"os"

	"github.com/chenen3/yeager/cert"
	"gopkg.in/yaml.v3"
)

type Config struct {
	SOCKSListen   string         `yaml:"socksListen,omitempty"`
	HTTPListen    string         `yaml:"httpListen,omitempty"`
	TunnelListens []TunnelListen `yaml:"tunnelListens,omitempty"`
	TunnelClients []TunnelClient `yaml:"tunnelClients,omitempty"`
	Rules         []string       `yaml:"rules,omitempty"`
	// verbose logging
	Verbose bool `yaml:"verbose,omitempty"`
	// enable HTTP server for debugging and profiling, developers only
	Debug bool `yaml:"debug,omitempty"`
}

type TunnelType string

const (
	TunGRPC TunnelType = "grpc"
	TunQUIC TunnelType = "quic"
)

type TunnelListen struct {
	Type     TunnelType `yaml:"type"`
	Listen   string     `yaml:"listen"`
	CertFile string     `yaml:"certFile,omitempty"`
	CertPEM  string     `yaml:"certPEM,omitempty"`
	KeyFile  string     `yaml:"keyFile,omitempty"`
	KeyPEM   string     `yaml:"keyPEM,omitempty"`
	CAFile   string     `yaml:"caFile,omitempty"`
	CAPEM    string     `yaml:"caPEM,omitempty"`
}

type TunnelClient struct {
	Type    TunnelType `yaml:"type"`
	Policy  string     `yaml:"policy"`
	Address string     `yaml:"address"` // target server address

	CertFile string `yaml:"certFile,omitempty"`
	CertPEM  string `yaml:"certPEM,omitempty"`
	KeyFile  string `yaml:"keyFile,omitempty"`
	KeyPEM   string `yaml:"keyPEM,omitempty"`
	CAFile   string `yaml:"caFile,omitempty"`
	CAPEM    string `yaml:"caPEM,omitempty"`

	// optional
	ConnectionPoolSize int `yaml:"connectionPoolSize,omitempty"`
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
				CAPEM:   string(cert.RootCert),
				CertPEM: string(cert.ServerCert),
				KeyPEM:  string(cert.ServerKey),
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
				CAPEM:   string(cert.RootCert),
				CertPEM: string(cert.ClientCert),
				KeyPEM:  string(cert.ClientKey),
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
	bs, err := yaml.Marshal(srvConf)
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
	// On client side, verbose logging helps to see where traffic is being sent to
	cliConf.Verbose = true
	bs, err = yaml.Marshal(cliConf)
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

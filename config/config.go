package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/chenen3/yeager/cert"
)

// tunnel type
const (
	TunGRPC  = "grpc"
	TunQUIC  = "quic"
	TunHTTP2 = "http2"
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
	// enable debug logging, start HTTP server for profiling
	Debug bool `json:"debug,omitempty"`
}

func mergeLines(s []string) string {
	return strings.Join(s, "\n")
}

func splitLines(s string) []string {
	return strings.Split(strings.TrimSpace(s), "\n")
}

type TunnelListen struct {
	Type     string   `json:"type"`
	Listen   string   `json:"listen"`
	CertFile string   `json:"certFile,omitempty"`
	CertPEM  []string `json:"certPEM,omitempty"`
	KeyFile  string   `json:"keyFile,omitempty"`
	KeyPEM   []string `json:"keyPEM,omitempty"`
	CAFile   string   `json:"caFile,omitempty"`
	CAPEM    []string `json:"caPEM,omitempty"`
}

func (tl *TunnelListen) GetCertPEM() ([]byte, error) {
	if tl.CertPEM != nil {
		return []byte(mergeLines(tl.CertPEM)), nil
	}
	if tl.CertFile != "" {
		return os.ReadFile(tl.CertFile)
	}
	return nil, errors.New("no PEM data nor file")
}

func (tl *TunnelListen) GetKeyPEM() ([]byte, error) {
	if tl.KeyPEM != nil {
		return []byte(mergeLines(tl.KeyPEM)), nil
	}
	if tl.KeyFile != "" {
		return os.ReadFile(tl.KeyFile)
	}
	return nil, errors.New("no PEM data nor file")
}

func (tl *TunnelListen) GetCAPEM() ([]byte, error) {
	if tl.CAPEM != nil {
		return []byte(mergeLines(tl.CAPEM)), nil
	}
	if tl.CAFile != "" {
		return os.ReadFile(tl.CAFile)
	}
	return nil, errors.New("no PEM data nor file")
}

type TunnelClient struct {
	Policy   string   `json:"policy"`
	Type     string   `json:"type"`
	Address  string   `json:"address"` // target server address
	CertFile string   `json:"certFile,omitempty"`
	CertPEM  []string `json:"certPEM,omitempty"`
	KeyFile  string   `json:"keyFile,omitempty"`
	KeyPEM   []string `json:"keyPEM,omitempty"`
	CAFile   string   `json:"caFile,omitempty"`
	CAPEM    []string `json:"caPEM,omitempty"`

	MaxStreamsPerConn int `json:"maxStreamsPerConn,omitempty"`
}

func (tc *TunnelClient) GetCertPEM() ([]byte, error) {
	if tc.CertPEM != nil {
		return []byte(mergeLines(tc.CertPEM)), nil
	}
	if tc.CertFile != "" {
		return os.ReadFile(tc.CertFile)
	}
	return nil, errors.New("no PEM data nor file")
}

func (tc *TunnelClient) GetKeyPEM() ([]byte, error) {
	if tc.KeyPEM != nil {
		return []byte(mergeLines(tc.KeyPEM)), nil
	}
	if tc.KeyFile != "" {
		return os.ReadFile(tc.KeyFile)
	}
	return nil, errors.New("no PEM data nor file")
}

func (tc *TunnelClient) GetCAPEM() ([]byte, error) {
	if tc.CAPEM != nil {
		return []byte(mergeLines(tc.CAPEM)), nil
	}
	if tc.CAFile != "" {
		return os.ReadFile(tc.CAFile)
	}
	return nil, errors.New("no PEM data nor file")
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
				CAPEM:   splitLines(string(cert.RootCert)),
				CertPEM: splitLines(string(cert.ServerCert)),
				KeyPEM:  splitLines(string(cert.ServerKey)),
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
				CAPEM:   splitLines(string(cert.RootCert)),
				CertPEM: splitLines(string(cert.ClientCert)),
				KeyPEM:  splitLines(string(cert.ClientKey)),
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
	port := 57175
	srvConf.TunnelListens[0].Listen = fmt.Sprintf("0.0.0.0:%d", port)
	bs, err := json.MarshalIndent(srvConf, "", "\t")
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
	cliConf.TunnelClients[0].Address = fmt.Sprintf("%s:%d", ip, port)
	cliConf.SOCKSListen = "127.0.0.1:1080"
	cliConf.HTTPListen = "127.0.0.1:8080"
	cliConf.Rules = []string{
		"ip-cidr,127.0.0.1/8,direct",
		"ip-cidr,192.168.0.0/16,direct",
		"ip-cidr,172.16.0.0/12,direct",
		"ip-cidr,10.0.0.0/8,direct",
		"domain,localhost,direct",
		// "geosite,cn,direct",
		// "geosite,apple@cn,direct",
		"final,proxy",
	}
	bs, err = json.MarshalIndent(cliConf, "", "\t")
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

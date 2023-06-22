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
	if err := json.Unmarshal(bs, &c); err != nil {
		return c, err
	}
	// backward compatible
	for i := range c.TunnelClients {
		tc := &c.TunnelClients[i]
		if tc.Name == "" && tc.Policy != "" {
			tc.Name = tc.Policy
		}
	}
	return c, nil
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
	Name string `json:"name"`
	// deprecated, use Name instead
	Policy   string   `json:"policy,omitempty"`
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

// Generate generates a pair of client and server configuration for the given host
func Generate(host string) (cli, srv Config, err error) {
	cert, err := cert.Generate(host)
	if err != nil {
		return
	}
	tunnelPort, err := allocPort()
	if err != nil {
		return
	}

	srv = Config{
		TunnelListens: []TunnelListen{
			{
				Listen:  fmt.Sprintf("0.0.0.0:%d", tunnelPort),
				Type:    TunHTTP2,
				CAPEM:   splitLines(string(cert.RootCert)),
				CertPEM: splitLines(string(cert.ServerCert)),
				KeyPEM:  splitLines(string(cert.ServerKey)),
			},
		},
	}

	socksProxyPort, err := allocPort()
	if err != nil {
		return
	}
	httpProxyPort, err := allocPort()
	if err != nil {
		return
	}
	cli = Config{
		SOCKSListen: fmt.Sprintf("127.0.0.1:%d", socksProxyPort),
		HTTPListen:  fmt.Sprintf("127.0.0.1:%d", httpProxyPort),
		TunnelClients: []TunnelClient{
			{
				Name:    "proxy",
				Address: fmt.Sprintf("%s:%d", host, tunnelPort),
				Type:    TunHTTP2,
				CAPEM:   splitLines(string(cert.RootCert)),
				CertPEM: splitLines(string(cert.ClientCert)),
				KeyPEM:  splitLines(string(cert.ClientKey)),
			},
		},
		Rules: []string{"final,proxy"},
	}
	return cli, srv, nil
}

package main

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/chenen3/yeager/cert"
)

type Config struct {
	Listen      []ServerConfig `json:"listen,omitempty"`
	Proxy       []ClientConfig `json:"proxy,omitempty"`
	Rules       []string       `json:"rules,omitempty"`
	ListenSOCKS string         `json:"listenSOCKS,omitempty"`
	ListenHTTP  string         `json:"listenHTTP,omitempty"`
}

const (
	ProtoGRPC  = "grpc"
	ProtoQUIC  = "quic"
	ProtoHTTP2 = "http2"
)

type ServerConfig struct {
	Proto   string `json:"proto"`
	Address string `json:"address"`

	// not required when using mutual TLS
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`

	CertFile string   `json:"certFile,omitempty"`
	CertPEM  []string `json:"certPEM,omitempty"`
	KeyFile  string   `json:"keyFile,omitempty"`
	KeyPEM   []string `json:"keyPEM,omitempty"`
	CAFile   string   `json:"caFile,omitempty"`
	CAPEM    []string `json:"caPEM,omitempty"`
}

func mergeLine(s []string) string {
	return strings.Join(s, "\n")
}

func splitLine(s string) []string {
	return strings.Split(strings.TrimSpace(s), "\n")
}

func (c ServerConfig) GetCertPEM() ([]byte, error) {
	if c.CertPEM != nil {
		return []byte(mergeLine(c.CertPEM)), nil
	}
	if c.CertFile != "" {
		return os.ReadFile(c.CertFile)
	}
	return nil, errors.New("no PEM data nor file")
}

func (c ServerConfig) GetKeyPEM() ([]byte, error) {
	if c.KeyPEM != nil {
		return []byte(mergeLine(c.KeyPEM)), nil
	}
	if c.KeyFile != "" {
		return os.ReadFile(c.KeyFile)
	}
	return nil, errors.New("no PEM data nor file")
}

func (c ServerConfig) GetCAPEM() ([]byte, error) {
	if c.CAPEM != nil {
		return []byte(mergeLine(c.CAPEM)), nil
	}
	if c.CAFile != "" {
		return os.ReadFile(c.CAFile)
	}
	return nil, errors.New("no PEM data nor file")
}

type ClientConfig struct {
	Name string `json:"name"`
	ServerConfig
}

func allocPort() int {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		// the function is intended to be called from command line,
		// panic is ok.
		panic(err)
	}
	ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

// GenerateConfig generates a pair of client and server configuration for the given host
func GenerateConfig(host string) (cli, srv Config, err error) {
	cert, err := cert.Generate(host)
	if err != nil {
		return
	}
	tunnelPort := allocPort()

	srv = Config{
		Listen: []ServerConfig{
			{
				Address: fmt.Sprintf("0.0.0.0:%d", tunnelPort),
				Proto:   ProtoGRPC,
				CAPEM:   splitLine(string(cert.RootCert)),
				CertPEM: splitLine(string(cert.ServerCert)),
				KeyPEM:  splitLine(string(cert.ServerKey)),
			},
		},
	}

	cli = Config{
		ListenSOCKS: fmt.Sprintf("127.0.0.1:%d", allocPort()),
		ListenHTTP:  fmt.Sprintf("127.0.0.1:%d", allocPort()),
		Proxy: []ClientConfig{
			{
				Name: "proxy",
				ServerConfig: ServerConfig{
					Address: fmt.Sprintf("%s:%d", host, tunnelPort),
					Proto:   ProtoGRPC,
					CAPEM:   splitLine(string(cert.RootCert)),
					CertPEM: splitLine(string(cert.ClientCert)),
					KeyPEM:  splitLine(string(cert.ClientKey)),
				}},
		},
		Rules: []string{"final,proxy"},
	}
	return cli, srv, nil
}

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
	Listen      []TransportConfig `json:"listen,omitempty"`
	ListenSOCKS string            `json:"listenSOCKS,omitempty"`
	ListenHTTP  string            `json:"listenHTTP,omitempty"`
	Proxy       TransportConfig   `json:"proxy,omitempty"`
}

const (
	ProtoGRPC  = "grpc"
	ProtoHTTP2 = "h2"
)

type TransportConfig struct {
	Proto    string   `json:"proto"`
	Address  string   `json:"address"`
	CertFile string   `json:"certFile,omitempty"`
	CertPEM  []string `json:"certPEM,omitempty"`
	KeyFile  string   `json:"keyFile,omitempty"`
	KeyPEM   []string `json:"keyPEM,omitempty"`
	CAFile   string   `json:"caFile,omitempty"`
	CAPEM    []string `json:"caPEM,omitempty"`

	// not required when using mutual TLS
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`

	allowPrivate bool // test only
}

func mergeLine(s []string) string {
	return strings.Join(s, "\n")
}

func splitLine(s string) []string {
	return strings.Split(strings.TrimSpace(s), "\n")
}

func (c TransportConfig) GetCertPEM() ([]byte, error) {
	if c.CertPEM != nil {
		return []byte(mergeLine(c.CertPEM)), nil
	}
	if c.CertFile != "" {
		return os.ReadFile(c.CertFile)
	}
	return nil, errors.New("no PEM data nor file")
}

func (c TransportConfig) GetKeyPEM() ([]byte, error) {
	if c.KeyPEM != nil {
		return []byte(mergeLine(c.KeyPEM)), nil
	}
	if c.KeyFile != "" {
		return os.ReadFile(c.KeyFile)
	}
	return nil, errors.New("no PEM data nor file")
}

func (c TransportConfig) GetCAPEM() ([]byte, error) {
	if c.CAPEM != nil {
		return []byte(mergeLine(c.CAPEM)), nil
	}
	if c.CAFile != "" {
		return os.ReadFile(c.CAFile)
	}
	return nil, errors.New("no PEM data nor file")
}

func allocPort() int {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
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
		Listen: []TransportConfig{
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
		Proxy: TransportConfig{
			Address: fmt.Sprintf("%s:%d", host, tunnelPort),
			Proto:   ProtoGRPC,
			CAPEM:   splitLine(string(cert.RootCert)),
			CertPEM: splitLine(string(cert.ClientCert)),
			KeyPEM:  splitLine(string(cert.ClientKey)),
		},
	}
	return cli, srv, nil
}

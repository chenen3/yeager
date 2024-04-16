package config

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
)

type Config struct {
	Listen []Transport `json:"listen,omitempty"`

	Transport Transport `json:"transport,omitempty"`
	// candidate transport for automatic switching
	Transports []Transport `json:"transports,omitempty"`
	SOCKSProxy string      `json:"socks_proxy,omitempty"`
	HTTPProxy  string      `json:"http_proxy,omitempty"`
}

const (
	ProtoGRPC  = "grpc"
	ProtoHTTP2 = "h2"
)

type Transport struct {
	Protocol string   `json:"protocol"`
	Address  string   `json:"address"`
	CertFile string   `json:"cert_file,omitempty"`
	CertPEM  []string `json:"cert_pem,omitempty"`
	KeyFile  string   `json:"key_file,omitempty"`
	KeyPEM   []string `json:"key_pem,omitempty"`
	CAFile   string   `json:"ca_file,omitempty"`
	CAPEM    []string `json:"ca_pem,omitempty"`

	// not required when using mutual TLS
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
}

func mergeLines(s []string) string {
	return strings.Join(s, "\n")
}

func splitLines(s string) []string {
	return strings.Split(strings.TrimSpace(s), "\n")
}

func (t Transport) Cert() ([]byte, error) {
	if t.CertPEM != nil {
		return []byte(mergeLines(t.CertPEM)), nil
	}
	if t.CertFile != "" {
		return os.ReadFile(t.CertFile)
	}
	return nil, errors.New("no PEM data nor file")
}

func (t Transport) Key() ([]byte, error) {
	if t.KeyPEM != nil {
		return []byte(mergeLines(t.KeyPEM)), nil
	}
	if t.KeyFile != "" {
		return os.ReadFile(t.KeyFile)
	}
	return nil, errors.New("no PEM data nor file")
}

func (t Transport) CA() ([]byte, error) {
	if t.CAPEM != nil {
		return []byte(mergeLines(t.CAPEM)), nil
	}
	if t.CAFile != "" {
		return os.ReadFile(t.CAFile)
	}
	return nil, errors.New("no PEM data nor file")
}

func anyPort() int {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

// Generate generates a pair of client and server configuration for the given host
func Generate(host string) (cli, srv Config, err error) {
	cert, err := newCert(host)
	if err != nil {
		return
	}
	tunnelPort := anyPort()

	srv = Config{
		Listen: []Transport{
			{
				Address:  fmt.Sprintf("0.0.0.0:%d", tunnelPort),
				Protocol: ProtoGRPC,
				CAPEM:    splitLines(string(cert.rootCert)),
				CertPEM:  splitLines(string(cert.serverCert)),
				KeyPEM:   splitLines(string(cert.serverKey)),
			},
		},
	}

	cli = Config{
		SOCKSProxy: fmt.Sprintf("127.0.0.1:%d", anyPort()),
		HTTPProxy:  fmt.Sprintf("127.0.0.1:%d", anyPort()),
		Transport: Transport{
			Address:  fmt.Sprintf("%s:%d", host, tunnelPort),
			Protocol: ProtoGRPC,
			CAPEM:    splitLines(string(cert.rootCert)),
			CertPEM:  splitLines(string(cert.clientCert)),
			KeyPEM:   splitLines(string(cert.clientKey)),
		},
	}
	return cli, srv, nil
}

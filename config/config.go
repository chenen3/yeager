package config

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"strings"
)

type Config struct {
	Listen []Transport `json:"listen,omitempty"`

	Transport Transport `json:"transport,omitempty"`
	// alternative transport for automatic switching
	Transports []Transport `json:"transports,omitempty"`
	SOCKSProxy string      `json:"socks_proxy,omitempty"`
	HTTPProxy  string      `json:"http_proxy,omitempty"`
}

const (
	ProtoGRPC        = "grpc"
	ProtoHTTP2       = "h2"
	ProtoShadowsocks = "ss"
)

type Transport struct {
	Protocol string   `json:"protocol,omitempty"`
	Address  string   `json:"address,omitempty"`
	CertPEM  []string `json:"cert_pem,omitempty"`
	KeyPEM   []string `json:"key_pem,omitempty"`
	CAPEM    []string `json:"ca_pem,omitempty"`

	// for h2
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`

	// for shadowsocks
	Cipher string `json:"cipher,omitempty"`
	Secret string `json:"secret,omitempty"`
}

func mergeLines(s []string) string {
	return strings.Join(s, "\n")
}

func splitLines(s string) []string {
	return strings.Split(strings.TrimSpace(s), "\n")
}

func (t Transport) ClientTLS() (*tls.Config, error) {
	if t.CertPEM == nil {
		return nil, errors.New("no certificate")
	}
	cert := []byte(mergeLines(t.CertPEM))
	if t.KeyPEM == nil {
		return nil, errors.New("no key")
	}
	key := []byte(mergeLines(t.KeyPEM))
	if t.CAPEM == nil {
		return nil, errors.New("no CA")
	}
	ca := []byte(mergeLines(t.CAPEM))
	return newClientTLS(ca, cert, key)
}

func (t Transport) ServerTLS() (*tls.Config, error) {
	if t.CertPEM == nil {
		return nil, errors.New("no certificate")
	}
	cert := []byte(mergeLines(t.CertPEM))
	if t.KeyPEM == nil {
		return nil, errors.New("no key")
	}
	key := []byte(mergeLines(t.KeyPEM))
	if t.CAPEM == nil {
		return nil, errors.New("no CA")
	}
	ca := []byte(mergeLines(t.CAPEM))
	return newServerTLS(ca, cert, key)
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

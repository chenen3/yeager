package config

import (
	"crypto/tls"
	"errors"
	"fmt"
	"strings"
)

type Config struct {
	Listen    []ServerConfig `json:"listen,omitempty"`    // supports http, socks5, grpc and h2 protocols
	Transport []ServerConfig `json:"transport,omitempty"` // supports grpc, h2 and shadowsocks protocols
}

const (
	ProtoHTTP   = "http"
	ProtoSOCKS5 = "socks5"

	ProtoGRPC        = "grpc"
	ProtoHTTP2       = "h2"
	ProtoShadowsocks = "ss"
)

type ServerConfig struct {
	Protocol string `json:"protocol,omitempty"`
	Address  string `json:"address,omitempty"`

	// for TLS
	CertPEM []string `json:"cert_pem,omitempty"`
	KeyPEM  []string `json:"key_pem,omitempty"`
	CAPEM   []string `json:"ca_pem,omitempty"`

	// for h2
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`

	// for shadowsocks
	Cipher string `json:"cipher,omitempty"`
	Secret string `json:"secret,omitempty"`
}

func mergeLine(s []string) string {
	return strings.Join(s, "\n")
}

func splitLine(s string) []string {
	return strings.Split(strings.TrimSpace(s), "\n")
}

func (s ServerConfig) ClientTLS() (*tls.Config, error) {
	if s.CertPEM == nil {
		return nil, errors.New("no certificate")
	}
	cert := []byte(mergeLine(s.CertPEM))
	if s.KeyPEM == nil {
		return nil, errors.New("no key")
	}
	key := []byte(mergeLine(s.KeyPEM))
	if s.CAPEM == nil {
		return nil, errors.New("no CA")
	}
	ca := []byte(mergeLine(s.CAPEM))
	return newClientTLSConfig(ca, cert, key)
}

func (s ServerConfig) ServerTLS() (*tls.Config, error) {
	if s.CertPEM == nil {
		return nil, errors.New("no certificate")
	}
	cert := []byte(mergeLine(s.CertPEM))
	if s.KeyPEM == nil {
		return nil, errors.New("no key")
	}
	key := []byte(mergeLine(s.KeyPEM))
	if s.CAPEM == nil {
		return nil, errors.New("no CA")
	}
	ca := []byte(mergeLine(s.CAPEM))
	return newServerTLSConfig(ca, cert, key)
}

// Generate returns a pair of client and server configuration for the given host
func Generate(host string) (cli, srv Config, err error) {
	cert, err := newCert(host)
	if err != nil {
		return
	}

	proxyPort := 57175
	srv = Config{
		Listen: []ServerConfig{
			{
				Address:  fmt.Sprintf("0.0.0.0:%d", proxyPort),
				Protocol: ProtoGRPC,
				CAPEM:    splitLine(string(cert.rootCert)),
				CertPEM:  splitLine(string(cert.serverCert)),
				KeyPEM:   splitLine(string(cert.serverKey)),
			},
		},
	}

	cli = Config{
		Listen: []ServerConfig{
			{Protocol: ProtoHTTP, Address: "127.0.0.1:8080"},
			{Protocol: ProtoSOCKS5, Address: "127.0.0.1:1080"},
		},
		Transport: []ServerConfig{
			{
				Address:  fmt.Sprintf("%s:%d", host, proxyPort),
				Protocol: ProtoGRPC,
				CAPEM:    splitLine(string(cert.rootCert)),
				CertPEM:  splitLine(string(cert.clientCert)),
				KeyPEM:   splitLine(string(cert.clientKey)),
			},
		},
	}
	return cli, srv, nil
}

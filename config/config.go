package config

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/chenen3/yeager/cert"
)

const (
	ProtoGRPC  = "grpc"
	ProtoQUIC  = "quic"
	ProtoHTTP2 = "http2"
)

type Config struct {
	ListenSOCKS string         `json:"listenSOCKS,omitempty"`
	ListenHTTP  string         `json:"listenHTTP,omitempty"`
	Listen      []TunnelServer `json:"listen,omitempty"`
	Proxy       []TunnelClient `json:"proxy,omitempty"`
	Rules       []string       `json:"rules,omitempty"`
	// enable debug logging, start HTTP server for profiling
	Debug bool `json:"debug,omitempty"`
}

func mergeLine(s []string) string {
	return strings.Join(s, "\n")
}

func splitLine(s string) []string {
	return strings.Split(strings.TrimSpace(s), "\n")
}

type TunnelServer struct {
	Proto    string   `json:"proto"`
	Address  string   `json:"address"`
	CertFile string   `json:"certFile,omitempty"`
	CertPEM  []string `json:"certPEM,omitempty"`
	KeyFile  string   `json:"keyFile,omitempty"`
	KeyPEM   []string `json:"keyPEM,omitempty"`
	CAFile   string   `json:"caFile,omitempty"`
	CAPEM    []string `json:"caPEM,omitempty"`
	Obfs     bool     `json:"obfs,omitempty"`
}

func (t TunnelServer) GetCertPEM() ([]byte, error) {
	if t.CertPEM != nil {
		return []byte(mergeLine(t.CertPEM)), nil
	}
	if t.CertFile != "" {
		return os.ReadFile(t.CertFile)
	}
	return nil, errors.New("no PEM data nor file")
}

func (t TunnelServer) GetKeyPEM() ([]byte, error) {
	if t.KeyPEM != nil {
		return []byte(mergeLine(t.KeyPEM)), nil
	}
	if t.KeyFile != "" {
		return os.ReadFile(t.KeyFile)
	}
	return nil, errors.New("no PEM data nor file")
}

func (t TunnelServer) GetCAPEM() ([]byte, error) {
	if t.CAPEM != nil {
		return []byte(mergeLine(t.CAPEM)), nil
	}
	if t.CAFile != "" {
		return os.ReadFile(t.CAFile)
	}
	return nil, errors.New("no PEM data nor file")
}

type TunnelClient struct {
	Name string `json:"name"`
	TunnelServer
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
		Listen: []TunnelServer{
			{
				Address: fmt.Sprintf("0.0.0.0:%d", tunnelPort),
				Proto:   ProtoHTTP2,
				CAPEM:   splitLine(string(cert.RootCert)),
				CertPEM: splitLine(string(cert.ServerCert)),
				KeyPEM:  splitLine(string(cert.ServerKey)),
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
		ListenSOCKS: fmt.Sprintf("127.0.0.1:%d", socksProxyPort),
		ListenHTTP:  fmt.Sprintf("127.0.0.1:%d", httpProxyPort),
		Proxy: []TunnelClient{
			{
				Name: "proxy",
				TunnelServer: TunnelServer{
					Address: fmt.Sprintf("%s:%d", host, tunnelPort),
					Proto:   ProtoHTTP2,
					CAPEM:   splitLine(string(cert.RootCert)),
					CertPEM: splitLine(string(cert.ClientCert)),
					KeyPEM:  splitLine(string(cert.ClientKey)),
				}},
		},
		Rules: []string{"final,proxy"},
	}
	return cli, srv, nil
}

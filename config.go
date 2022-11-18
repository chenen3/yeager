package main

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/chenen3/yeager/config"
	"github.com/chenen3/yeager/util"
	"gopkg.in/yaml.v3"
)

func readDataOrFile(data string, filename string) ([]byte, error) {
	if data != "" {
		return []byte(data), nil
	}
	if filename != "" {
		return os.ReadFile(filename)
	}
	return nil, errors.New("no data nor filename provided")
}

// make server config for mutual TLS
func makeServerTLSConfig(tl config.TunnelListen) (*tls.Config, error) {
	certPEM, err := readDataOrFile(tl.CertPEM, tl.CertFile)
	if err != nil {
		return nil, fmt.Errorf("read TLS certificate: %s", err)
	}
	keyPEM, err := readDataOrFile(tl.KeyPEM, tl.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("read TLS key: %s", err)
	}
	caPEM, err := readDataOrFile(tl.CAPEM, tl.CAFile)
	if err != nil {
		return nil, fmt.Errorf("read TLS CA: %s", err)
	}
	tlsConf := &tls.Config{
		MinVersion: tls.VersionTLS13,
	}
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, errors.New("parse cert pem: " + err.Error())
	}
	tlsConf.Certificates = []tls.Certificate{cert}

	pool := x509.NewCertPool()
	ok := pool.AppendCertsFromPEM(caPEM)
	if !ok {
		return nil, errors.New("failed to parse root cert pem")
	}
	tlsConf.ClientCAs = pool
	tlsConf.ClientAuth = tls.RequireAndVerifyClientCert
	return tlsConf, nil
}

// make client config for mutual TLS
func makeClientTLSConfig(tl config.TunnelClient) (*tls.Config, error) {
	certPEM, err := readDataOrFile(tl.CertPEM, tl.CertFile)
	if err != nil {
		return nil, fmt.Errorf("read TLS certificate: %s", err)
	}
	keyPEM, err := readDataOrFile(tl.KeyPEM, tl.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("read TLS key: %s", err)
	}
	caPEM, err := readDataOrFile(tl.CAPEM, tl.CAFile)
	if err != nil {
		return nil, fmt.Errorf("read TLS CA: %s", err)
	}
	tlsConf := &tls.Config{
		ClientSessionCache: tls.NewLRUClientSessionCache(64),
	}
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, err
	}
	tlsConf.Certificates = []tls.Certificate{cert}
	pool := x509.NewCertPool()
	if ok := pool.AppendCertsFromPEM(caPEM); !ok {
		return nil, errors.New("failed to parse root certificate")
	}
	tlsConf.RootCAs = pool
	return tlsConf, nil
}

// make the corresponding client and server config
func makeConfig(host string) (srv, cli config.Config, err error) {
	cert, err := util.GenerateCertificate(host)
	if err != nil {
		return srv, cli, err
	}
	tunnelPort, err := util.AllocatePort()
	if err != nil {
		return srv, cli, err
	}

	srv = config.Config{
		TunnelListens: []config.TunnelListen{
			{
				Listen:  fmt.Sprintf("0.0.0.0:%d", tunnelPort),
				Type:    config.TunGRPC,
				CAPEM:   string(cert.RootCert),
				CertPEM: string(cert.ServerCert),
				KeyPEM:  string(cert.ServerKey),
			},
		},
		Rules: []string{"final,direct"},
	}

	socksProxyPort, err := util.AllocatePort()
	if err != nil {
		return srv, cli, err
	}
	httpProxyPort, err := util.AllocatePort()
	if err != nil {
		return srv, cli, err
	}
	cli = config.Config{
		SOCKSListen: fmt.Sprintf("127.0.0.1:%d", socksProxyPort),
		HTTPListen:  fmt.Sprintf("127.0.0.1:%d", httpProxyPort),
		TunnelClients: []config.TunnelClient{
			{
				Policy:  "proxy",
				Address: fmt.Sprintf("%s:%d", host, tunnelPort),
				Type:    config.TunGRPC,
				CAPEM:   string(cert.RootCert),
				CertPEM: string(cert.ClientCert),
				KeyPEM:  string(cert.ClientKey),
			},
		},
		Rules: []string{"final,proxy"},
	}
	return srv, cli, nil
}

// GenerateConfig generates the corrensponding client and server config file
func GenerateConfig(ip, srvConfOutput, cliConfOutput string) error {
	_, err := os.Stat(srvConfOutput)
	if err == nil {
		return fmt.Errorf("found %s, operation aborted", srvConfOutput)
	}
	_, err = os.Stat(cliConfOutput)
	if err == nil {
		return fmt.Errorf("found %s, operation aborted", cliConfOutput)
	}

	srvConf, cliConf, err := makeConfig(ip)
	if err != nil {
		return fmt.Errorf("failed to generate config: %s", err)
	}
	if len(srvConf.TunnelListens) == 0 {
		return fmt.Errorf("no tunnelListens in server config")
	}
	srvConf.TunnelListens[0].Listen = "0.0.0.0:9001"
	srvConf.Rules = []string{
		"ip-cidr,127.0.0.1/8,reject",
		"ip-cidr,192.168.0.0/16,reject",
		"domain,localhost,reject",
		"final,direct",
	}
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

func publicIP() (string, error) {
	resp, err := http.Get("https://checkip.amazonaws.com")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	ip, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	ip = bytes.TrimSpace(ip)
	return string(ip), nil
}

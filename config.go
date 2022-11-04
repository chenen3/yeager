package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/chenen3/yeager/config"
	"github.com/chenen3/yeager/util"
	"gopkg.in/yaml.v3"
)

func makeConfig(host string) (srv, cli config.Config, err error) {
	cert, err := util.GenerateCertificate(host)
	if err != nil {
		return srv, cli, err
	}
	tunnelPort, err := util.ChoosePort()
	if err != nil {
		return srv, cli, err
	}

	srv = config.Config{
		Inbounds: []config.YeagerServer{
			{
				Listen:    fmt.Sprintf(":%d", tunnelPort),
				Transport: config.TransGRPC,
				TLS: config.TLS{
					CAPEM:   string(cert.RootCert),
					CertPEM: string(cert.ServerCert),
					KeyPEM:  string(cert.ServerKey),
				},
			},
		},
		Rules: []string{"final,direct"},
	}

	socksProxyPort, err := util.ChoosePort()
	if err != nil {
		return srv, cli, err
	}
	httpProxyPort, err := util.ChoosePort()
	if err != nil {
		return srv, cli, err
	}
	cli = config.Config{
		SOCKSListen: fmt.Sprintf("127.0.0.1:%d", socksProxyPort),
		HTTPListen:  fmt.Sprintf("127.0.0.1:%d", httpProxyPort),
		Outbounds: []config.YeagerClient{
			{
				Tag:       "proxy",
				Address:   fmt.Sprintf("%s:%d", host, tunnelPort),
				Transport: config.TransGRPC,
				TLS: config.TLS{
					CAPEM:   string(cert.RootCert),
					CertPEM: string(cert.ClientCert),
					KeyPEM:  string(cert.ClientKey),
				},
			},
		},
		Rules: []string{"final,proxy"},
	}
	return srv, cli, nil
}

func GenerateConfig(ip, srvConfFile, cliConfFile string) error {
	_, err := os.Stat(srvConfFile)
	if err == nil {
		return fmt.Errorf("found %s, operation aborted", srvConfFile)
	}
	_, err = os.Stat(cliConfFile)
	if err == nil {
		return fmt.Errorf("found %s, operation aborted", cliConfFile)
	}

	srvConf, cliConf, err := makeConfig(ip)
	if err != nil {
		return fmt.Errorf("failed to generate config: %s", err)
	}
	if len(srvConf.Inbounds) == 0 {
		return fmt.Errorf("no inbound in server config")
	}
	srvConf.Inbounds[0].Listen = "0.0.0.0:9001"
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
	err = os.WriteFile(srvConfFile, bs, 0644)
	if err != nil {
		return fmt.Errorf("failed to write server config: %s", err)
	}
	fmt.Printf("generated server config file: %s\n", srvConfFile)

	if len(cliConf.Outbounds) == 0 {
		return fmt.Errorf("no outbound in client config")
	}
	cliConf.Outbounds[0].Address = fmt.Sprintf("%s:%d", ip, 9001)
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
	err = os.WriteFile(cliConfFile, bs, 0644)
	if err != nil {
		return fmt.Errorf("failed to write client config: %s", err)
	}
	fmt.Printf("generated client config file: %s\n", cliConfFile)
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

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/chenen3/yeager/logger"
)

var version string // set by build -ldflags

func checkIP() (string, error) {
	resp, err := http.Get("https://checkip.amazonaws.com")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	ip, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(ip)), nil
}

func genConfig(host, cliConfOutput, srvConfOutput string) error {
	if _, err := os.Stat(srvConfOutput); err == nil {
		return fmt.Errorf("file %s already exists, operation aborted", srvConfOutput)
	}
	if _, err := os.Stat(cliConfOutput); err == nil {
		return fmt.Errorf("file %s already exists, operation aborted", cliConfOutput)
	}

	cliConf, srvConf, err := GenerateConfig(host)
	if err != nil {
		return fmt.Errorf("failed to generate config: %s", err)
	}
	port := 57175
	srvConf.Listen[0].Address = fmt.Sprintf("0.0.0.0:%d", port)
	bs, err := json.MarshalIndent(srvConf, "", "\t")
	if err != nil {
		return fmt.Errorf("failed to marshal server config: %s", err)
	}
	if err = os.WriteFile(srvConfOutput, bs, 0644); err != nil {
		return fmt.Errorf("failed to write server config: %s", err)
	}
	fmt.Println("generated", srvConfOutput)

	cliConf.Proxy.Address = fmt.Sprintf("%s:%d", host, port)
	cliConf.ListenSOCKS = "127.0.0.1:1080"
	cliConf.ListenHTTP = "127.0.0.1:8080"
	bs, err = json.MarshalIndent(cliConf, "", "\t")
	if err != nil {
		return fmt.Errorf("failed to marshal client config: %s", err)
	}
	err = os.WriteFile(cliConfOutput, bs, 0644)
	if err != nil {
		return fmt.Errorf("failed to write client config: %s", err)
	}
	fmt.Println("generated", cliConfOutput)
	return nil
}

func main() {
	var flags struct {
		configFile string
		version    bool
		genConfig  bool
		ip         string
		debug      bool
	}
	flag.StringVar(&flags.configFile, "config", "", "path to configuration file")
	flag.BoolVar(&flags.version, "version", false, "print version")
	flag.BoolVar(&flags.genConfig, "genconf", false, "generate a pair of config: client.json and server.json")
	flag.StringVar(&flags.ip, "ip", "", "IP for the certificate, used with option -genconf")
	flag.BoolVar(&flags.debug, "debug", false, "start a local HTTP server for profiling")
	flag.Parse()

	if flags.version {
		fmt.Printf("yeager version %s\n", version)
		return
	}

	if flags.genConfig {
		ip := flags.ip
		if ip == "" {
			i, err := checkIP()
			if err != nil {
				fmt.Printf("get public IP: %s\n", err)
				return
			}
			ip = i
		}
		if err := genConfig(ip, "client.json", "server.json"); err != nil {
			fmt.Println(err)
			return
		}
		return
	}

	if flags.debug {
		s := http.Server{Addr: "localhost:6060"}
		defer s.Close()
		go func() {
			if err := s.ListenAndServe(); err != http.ErrServerClosed {
				logger.Error.Printf("debug server: %s", err)
			}
		}()
	}

	if flags.configFile == "" {
		fmt.Println("missing option -config")
		return
	}
	bs, err := os.ReadFile(flags.configFile)
	if err != nil {
		logger.Error.Printf("read config: %s", err)
		return
	}
	var conf Config
	if err = json.Unmarshal(bs, &conf); err != nil {
		logger.Error.Printf("load config: %s", err)
		return
	}

	// FYI
	logger.Info.Printf("yeager starting version: %s", version)
	if conf.Proxy.Address != "" {
		logger.Info.Printf("proxy server: %s %s", conf.Proxy.Proto, conf.Proxy.Address)
	}
	for _, sc := range conf.Listen {
		logger.Info.Printf("listen %s %s", sc.Proto, sc.Address)
	}

	services, err := StartServices(conf)
	if err != nil {
		logger.Error.Printf("start services: %s", err)
		closeAll(services)
		return
	}

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGTERM, syscall.SIGINT)
	sig := <-ch
	logger.Info.Printf("signal %s", sig)
	closeAll(services)
	logger.Info.Printf("goodbye")
}

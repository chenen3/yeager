package main

import (
	"encoding/json"
	_ "expvar"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/chenen3/yeager/config"
	"github.com/chenen3/yeager/debug"
	"github.com/chenen3/yeager/rule"
)

var version string // set by build -ldflags

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage of %s:\n", os.Args[0])
		flag.PrintDefaults()
		fmt.Fprint(flag.CommandLine.Output(), example)
	}
}

var example = `
Example:
  yeager -config /usr/local/etc/yeager/config.json
    	run service

  yeager -version
    	print version number

  yeager -genconf [-ip 1.2.3.4] [-cliconf client.json] [-srvconf config.json]
    	generate a pair of configuration for server and client
`

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
	_, err := os.Stat(srvConfOutput)
	if err == nil {
		return fmt.Errorf("file %s already exists, operation aborted", srvConfOutput)
	}
	_, err = os.Stat(cliConfOutput)
	if err == nil {
		return fmt.Errorf("file %s already exists, operation aborted", cliConfOutput)
	}

	cliConf, srvConf, err := config.Generate(host)
	if err != nil {
		return fmt.Errorf("failed to generate config: %s", err)
	}
	if len(srvConf.TunnelListens) == 0 {
		return fmt.Errorf("no tunnelListens in server config")
	}
	port := 57175
	srvConf.TunnelListens[0].Listen = fmt.Sprintf("0.0.0.0:%d", port)
	bs, err := json.MarshalIndent(srvConf, "", "\t")
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
	cliConf.TunnelClients[0].Address = fmt.Sprintf("%s:%d", host, port)
	cliConf.SOCKSListen = "127.0.0.1:1080"
	cliConf.HTTPListen = "127.0.0.1:8080"
	cliConf.Rules = []string{
		"ip-cidr,127.0.0.1/8,direct",
		"ip-cidr,192.168.0.0/16,direct",
		"ip-cidr,172.16.0.0/12,direct",
		"ip-cidr,10.0.0.0/8,direct",
		"domain,localhost,direct",
		"final,proxy",
	}
	bs, err = json.MarshalIndent(cliConf, "", "\t")
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

func main() {
	var flags struct {
		configFile  string
		cert        bool
		host        string
		version     bool
		genConfig   bool
		ip          string
		srvConfFile string
		cliConfFile string
	}
	flag.StringVar(&flags.configFile, "config", "", "path to configuration file")
	flag.BoolVar(&flags.version, "version", false, "print version")
	flag.BoolVar(&flags.genConfig, "genconf", false, "generate configuration")
	flag.StringVar(&flags.ip, "ip", "", "IP for the certificate, used with option -genconf")
	flag.StringVar(&flags.srvConfFile, "srvconf", "config.json", "server configuration file, used with option -genconf")
	flag.StringVar(&flags.cliConfFile, "cliconf", "client.json", "client configuration file, used with option -genconf")
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
		if err := genConfig(ip, flags.cliConfFile, flags.srvConfFile); err != nil {
			fmt.Println(err)
			return
		}
		return
	}

	if flags.configFile == "" {
		flag.Usage()
		return
	}
	bs, err := os.ReadFile(flags.configFile)
	if err != nil {
		log.Print(err)
		return
	}
	conf, err := config.Load(bs)
	if err != nil {
		log.Printf("load config: %s", err)
		return
	}
	if len(conf.TunnelClients) == 0 && len(conf.TunnelListens) == 0 {
		log.Printf("bad config: no tunnel client nor server")
		return
	}

	if conf.Debug {
		debug.Enable()
		go func() {
			log.Println(http.ListenAndServe("localhost:6060", nil))
		}()
	}

	log.Printf("yeager %s starting", version)
	closers, err := StartServices(conf)
	if err != nil {
		log.Printf("start services: %s", err)
		CloseAll(closers)
		return
	}
	rule.Cleanup()

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGTERM, syscall.SIGINT)
	log.Println("signal", <-ch)
	CloseAll(closers)
	log.Println("goodbye")
}

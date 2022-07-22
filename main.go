package main

import (
	"bytes"
	_ "expvar"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/chenen3/yeager/config"
	"gopkg.in/yaml.v3"
)

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage of %s:\n", os.Args[0])
		flag.PrintDefaults()
		fmt.Fprint(flag.CommandLine.Output(), example)
	}
}

// set by Github Action at project release
var version = "dev"

var example = `
Example:
  yeager -config /usr/local/etc/yeager/config.yaml
    	run service

  yeager -cert -host 127.0.0.1
    	generate certificates for mutual TLS

  yeager -version
    	print version number

  yeager -genconf [-srvconf server.yaml] [-cliconf client.yaml]
    	generate a pair of configuration for server and client
`

func main() {
	var flags struct {
		configFile  string
		cert        bool
		host        string
		version     bool
		genConf     bool
		srvConfFile string
		cliConfFile string
	}
	flag.StringVar(&flags.configFile, "config", "", "path to configuration file")
	flag.BoolVar(&flags.cert, "cert", false, "generate certificates")
	flag.StringVar(&flags.host, "host", "", "IP to generate a certificate for, used with option -cert")
	flag.BoolVar(&flags.version, "version", false, "print version")
	flag.BoolVar(&flags.genConf, "genconf", false, "generate configuration")
	flag.StringVar(&flags.srvConfFile, "srvconf", "server.yaml", "file name of server config, used with option -genconf")
	flag.StringVar(&flags.cliConfFile, "cliconf", "client.yaml", "file name of client config, used with option -genconf")
	flag.Parse()

	if flags.version {
		fmt.Printf("yeager version %s\n", version)
		return
	}

	if flags.genConf {
		err := generateConfig(flags.srvConfFile, flags.cliConfFile)
		if err != nil {
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
		log.Printf("failed to load config: %s", err)
		return
	}

	p, err := NewProxy(conf)
	if err != nil {
		log.Printf("init proxy: %s", err)
		return
	}
	// reduce the memory usage boosted by parsing rules of geosite.dat
	runtime.GC()

	// for profiling
	if conf.Debug {
		go func() {
			err := http.ListenAndServe("localhost:6060", nil)
			if err != nil {
				log.Printf("http server exit: %s", err)
			}
		}()
	}

	go func() {
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, syscall.SIGTERM, syscall.SIGINT)
		<-ch
		if err := p.Stop(); err != nil {
			panic("failed to stop service: " + err.Error())
		}
	}()
	log.Printf("yeager %s starting", version)
	p.Start()
	log.Print("service stopped")
}

func generateConfig(srvConfFile, cliConfFile string) error {
	_, err := os.Stat(srvConfFile)
	if err == nil {
		return fmt.Errorf("file already exists: %s, please specified another server config filename", srvConfFile)
	}
	_, err = os.Stat(cliConfFile)
	if err == nil {
		return fmt.Errorf("file already exists: %s, please specified another client config filename", cliConfFile)
	}

	resp, err := http.Get("https://checkip.amazonaws.com")
	if err != nil {
		return fmt.Errorf("failed to get plubic IP: %s", err)
	}
	defer resp.Body.Close()
	ip, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read IP: %s", err)
	}
	ip = bytes.TrimSpace(ip)
	srvConf, cliConf, err := GenerateConfig(string(ip))
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

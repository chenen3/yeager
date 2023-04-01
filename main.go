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

  yeager -genconf [-ip 1.2.3.4] [-srvconf server.json] [-cliconf client.json]
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
	ip = bytes.TrimSpace(ip)
	return string(ip), nil
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
	flag.StringVar(&flags.srvConfFile, "srvconf", "server.json", "file name of server config, used with option -genconf")
	flag.StringVar(&flags.cliConfFile, "cliconf", "client.json", "file name of client config, used with option -genconf")
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
		if err := config.Generate(ip, flags.srvConfFile, flags.cliConfFile); err != nil {
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
		log.Printf("at least one tunnel client or server is required in config")
		return
	}

	log.Printf("yeager %s starting", version)
	closers, err := StartServices(conf)
	if err != nil {
		log.Printf("start services: %s", err)
		CloseAll(closers)
		return
	}
	rule.Cleanup()

	if conf.Debug {
		debug.Enable()
		go func() {
			log.Println(http.ListenAndServe("localhost:6060", nil))
		}()
	}

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGTERM, syscall.SIGINT)
	log.Println("signal", <-ch)
	CloseAll(closers)
	log.Println("goodbye")
}

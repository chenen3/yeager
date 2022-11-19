package main

import (
	_ "expvar"
	"flag"
	"fmt"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"syscall"

	"gopkg.in/yaml.v3"

	"github.com/chenen3/yeager/config"
)

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage of %s:\n", os.Args[0])
		flag.PrintDefaults()
		fmt.Fprint(flag.CommandLine.Output(), example)
	}
}

// set by Github Action on release
var version = "dev"

var example = `
Example:
  yeager -config /usr/local/etc/yeager/config.yaml
    	run service

  yeager -version
    	print version number

  yeager -genconf [-ip 1.2.3.4] [-srvconf server.yaml] [-cliconf client.yaml]
    	generate a pair of configuration for server and client
`

func main() {
	var flags struct {
		configFile  string
		cert        bool
		host        string
		version     bool
		genConf     bool
		ip          string
		srvConfFile string
		cliConfFile string
	}
	flag.StringVar(&flags.configFile, "config", "", "path to configuration file")
	flag.BoolVar(&flags.version, "version", false, "print version")
	flag.BoolVar(&flags.genConf, "genconf", false, "generate configuration")
	flag.StringVar(&flags.ip, "ip", "", "IP for the certificate, used with option -genconf")
	flag.StringVar(&flags.srvConfFile, "srvconf", "server.yaml", "file name of server config, used with option -genconf")
	flag.StringVar(&flags.cliConfFile, "cliconf", "client.yaml", "file name of client config, used with option -genconf")
	flag.Parse()

	if flags.version {
		fmt.Printf("yeager version %s\n", version)
		return
	}

	// if flags.genConf {
	// 	ip := flags.ip
	// 	if ip == "" {
	// 		i, err := publicIP()
	// 		if err != nil {
	// 			fmt.Printf("get public IP: %s\n", err)
	// 			return
	// 		}
	// 		ip = i
	// 	}
	// 	err := GenerateConfig(ip, flags.srvConfFile, flags.cliConfFile)
	// 	if err != nil {
	// 		fmt.Println(err)
	// 		return
	// 	}
	// 	return
	// }

	if flags.configFile == "" {
		flag.Usage()
		return
	}
	bs, err := os.ReadFile(flags.configFile)
	if err != nil {
		log.Print(err)
		return
	}
	var conf config.Config
	err = yaml.Unmarshal(bs, &conf)
	if err != nil {
		log.Printf("failed to load config: %s", err)
		return
	}
	if len(conf.TunnelClients) == 0 && len(conf.TunnelListens) == 0 {
		log.Printf("config error: at least one tunnel client or server is required")
		return
	}

	log.Printf("yeager %s starting", version)
	closers, err := StartServices(conf)
	if err != nil {
		log.Printf("failed to start services: %s", err)
		CloseAll(closers)
		return
	}
	defer CloseAll(closers)

	if conf.Debug {
		go func() {
			err := http.ListenAndServe("localhost:6060", nil)
			if err != nil {
				log.Printf("http server exit: %s", err)
			}
		}()
	}

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGTERM, syscall.SIGINT)
	<-ch
	log.Print("bye bye")
}

package main

import (
	"encoding/json"
	_ "expvar"
	"flag"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/chenen3/yeager/config"
	"github.com/chenen3/yeager/log"
	"github.com/chenen3/yeager/proxy"
	"github.com/chenen3/yeager/util"
)

// set by compilation, for example:
// go build -ldflags="-X main.version=v0.1"
var version string

func printUsage() {
	flag.Usage()
	fmt.Print(`
Example:
  yeager -config /usr/local/etc/yeager/config.json
      run service

  yeager -cert -host 127.0.0.1
      generate certificates for mutual TLS

  yeager -version
      print version number
`)
}

func main() {
	var flags struct {
		configFile string
		cert       bool
		host       string
		version    bool
	}
	flag.StringVar(&flags.configFile, "config", "", "config file path")
	flag.BoolVar(&flags.cert, "cert", false, "generate certificates")
	flag.StringVar(&flags.host, "host", "", "comma-separated hostnames and IPs to generate a certificate for")
	flag.BoolVar(&flags.version, "version", false, "print version")
	flag.Parse()

	if flags.version {
		fmt.Printf("yeager version %s\n", version)
		return
	}

	if flags.cert {
		if flags.host == "" {
			fmt.Printf("ERROR: required flag \"-host\" not set\n")
			printUsage()
			return
		}

		_, err := util.GenerateCertificate(flags.host, true)
		if err != nil {
			fmt.Printf("failed to generate certificate: %s\n", err)
			return
		}

		fmt.Printf("generate certificate: \n\t%s\n\t%s\n\t%s\n\t%s\n\t%s\n\t%s\n",
			util.CACertFile, util.CAKeyFile,
			util.ServerCertFile, util.ServerKeyFile,
			util.ClientCertFile, util.ClientKeyFile,
		)
		fmt.Printf("please copy %s, %s, and %s to client device\n",
			util.CACertFile, util.ClientCertFile, util.ClientKeyFile,
		)
		return
	}

	if flags.configFile == "" {
		printUsage()
		return
	}

	conf, err := config.LoadFile(flags.configFile)
	if err != nil {
		log.Errorf("failed to load config: %s", err)
		return
	}
	if conf.Debug {
		bs, _ := json.MarshalIndent(conf, "", "  ")
		log.Infof("loaded config: \n%s", bs)
	}

	p, err := proxy.NewProxy(conf)
	if err != nil {
		log.Errorf("init proxy: %s", err)
		return
	}
	// trigger GC to release memory usage. (especially routing rule parsing)
	runtime.GC()
	// http server for profiling
	if conf.Debug {
		go func() {
			err := http.ListenAndServe("localhost:6060", nil)
			if err != nil {
				log.Errorf("http server exit: %s", err)
			}
		}()
	}

	go func() {
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, syscall.SIGTERM, syscall.SIGINT)
		<-ch
		// in case the server does not stop
		time.AfterFunc(3*time.Second, func() {
			os.Exit(1)
		})
		if err := p.Close(); err != nil {
			log.Errorf("failed to close proxy server: %s", err)
		}
	}()
	log.Infof("yeager %s starting", version)
	p.Serve()
	log.Infof("closing")
}

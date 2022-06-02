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
	"runtime"
	"syscall"

	"github.com/chenen3/yeager/config"
	"github.com/chenen3/yeager/util"
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
  yeager -config /usr/local/etc/yeager/config.json
      run service

  yeager -cert -host 127.0.0.1
      generate certificates for mutual TLS

  yeager -version
      print version number
`

func main() {
	var flags struct {
		configFile string
		cert       bool
		host       string
		version    bool
	}
	flag.StringVar(&flags.configFile, "config", "", "path to configuration file")
	flag.BoolVar(&flags.cert, "cert", false, "generate certificates")
	flag.StringVar(&flags.host, "host", "", "IP to generate a certificate for")
	flag.BoolVar(&flags.version, "version", false, "print version")
	flag.Parse()

	if flags.version {
		fmt.Printf("yeager version %s\n", version)
		return
	}

	if flags.cert {
		if flags.host == "" {
			fmt.Println("ERROR: required -host")
			flag.Usage()
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
	if conf.Verbose {
		bs, _ := yaml.Marshal(conf)
		log.Printf("loaded config: \n%s", bs)
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
		if err := p.Close(); err != nil {
			panic("failed to close: " + err.Error())
		}
	}()
	log.Printf("yeager %s starting", version)
	p.Serve()
}

package main

import (
	"encoding/json"
	"flag"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"yeager"
	"yeager/config"
	"yeager/log"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var confFile = flag.String("config", "/usr/local/etc/yeager/config.json", "config file")

func main() {
	flag.Parse()

	// try to load config from environment variables, if failed, then load from file
	conf := config.LoadEnv()
	if conf == nil {
		var err error
		conf, err = config.LoadFile(*confFile)
		if err != nil {
			log.Error(err)
			return
		}
	}
	bs, _ := json.MarshalIndent(conf, "", "  ")
	log.Infof("current configuration: \n%s\n", bs)

	p, err := yeager.NewProxy(conf)
	if err != nil {
		log.Error(err)
		return
	}
	// trigger GC to release memory usage. (especially routing rule parsing)
	runtime.GC()

	// http server for profiling
	go func() {
		http.Handle("/metrics", promhttp.Handler())
		log.Error(http.ListenAndServe("localhost:6060", nil))
	}()

	terminate := make(chan os.Signal, 1)
	signal.Notify(terminate, syscall.SIGTERM, os.Interrupt)
	log.Infof("starting ...")
	p.Start()

	// clean up
	<-terminate
	log.Infof("closing...")
	p.Close()
}

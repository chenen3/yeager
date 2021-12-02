package main

import (
	"encoding/json"
	_ "expvar"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/chenen3/yeager/config"
	"github.com/chenen3/yeager/log"
	"github.com/chenen3/yeager/proxy"
)

var confFile string

func init() {
	rootCmd.AddCommand(serveCmd)
	serveCmd.Flags().StringVarP(&confFile, "config", "c", "/usr/local/etc/yeager/config.json", "configuration file to read from")
}

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "serve client-side or server-side proxy",
	Run: func(cmd *cobra.Command, args []string) {
		serve()
	},
}

func serve() {
	// load config from environment variables or file
	conf, err, foundEnv := config.LoadEnv()
	if !foundEnv {
		log.L().Infof("loading config from %s", confFile)
		conf, err = config.LoadFile(confFile)
	}
	if err != nil {
		log.L().Errorf("load config: %s", err)
		return
	}
	bs, _ := json.MarshalIndent(conf, "", "  ")
	log.L().Infof("loaded config: \n%s", bs)

	p, err := proxy.NewProxy(conf)
	if err != nil {
		log.L().Errorf("init proxy: %s", err)
		return
	}
	// trigger GC to release memory usage. (especially routing rule parsing)
	runtime.GC()

	// http server for profiling
	if conf.Develop {
		go func() {
			http.ListenAndServe("localhost:6060", nil)
		}()
	}

	// clean up
	go func() {
		terminate := make(chan os.Signal, 1)
		signal.Notify(terminate, syscall.SIGTERM, os.Interrupt)
		<-terminate
		if err := p.Close(); err != nil {
			log.L().Errorf("close proxy: %s", err)
		}
	}()

	log.L().Infof("yeager %s starting ...", Version)
	p.Serve()
	log.L().Infof("closed")
}

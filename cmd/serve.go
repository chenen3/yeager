package cmd

import (
	"encoding/json"
	_ "expvar"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/chenen3/yeager/cmd/command"
	"github.com/chenen3/yeager/config"
	"github.com/chenen3/yeager/log"
	"github.com/chenen3/yeager/proxy"
)

var confFile string

func init() {
	Root.AddCommand(serveCmd)
	serveCmd.Flags().StringVar(&confFile, "config", "/usr/local/etc/yeager/config.json", "config file path")
}

var serveCmd = &command.Command{
	Name: "serve",
	Desc: "serve client-side or server-side proxy",
	Do: func(_ *command.Command) {
		log.L().Infof("load config file: %s", confFile)
		conf, err := config.LoadFile(confFile)
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
		if conf.Debug {
			go func() {
				log.L().Error(http.ListenAndServe("localhost:6060", nil))
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
	},
}

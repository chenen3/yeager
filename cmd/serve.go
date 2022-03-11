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
		conf, err := config.LoadFile(confFile)
		if err != nil {
			log.Errorf("load config: %s", err)
			return
		}
		if conf.Verbose {
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
				log.Error(http.ListenAndServe("localhost:6060", nil))
			}()
		}

		go func() {
			ch := make(chan os.Signal, 1)
			signal.Notify(ch, syscall.SIGTERM, syscall.SIGINT)
			<-ch
			if err := p.Close(); err != nil {
				log.Errorf("close proxy: %s", err)
			}
		}()
		log.Infof("yeager %s starting", Version)
		p.Serve()
	},
}

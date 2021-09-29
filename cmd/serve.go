package cmd

import (
	"encoding/json"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/chenen3/yeager/config"
	"github.com/chenen3/yeager/log"
	"github.com/chenen3/yeager/proxy"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/cobra"
)

var confFile string

func init() {
	rootCmd.AddCommand(serveCmd)
	serveCmd.Flags().StringVarP(&confFile, "config", "c", "/usr/local/etc/yeager/config.json", "Configuration file to read from")
	serveCmd.MarkFlagRequired("config")
}

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Serve client-side or server-side proxy",
	Run: func(cmd *cobra.Command, args []string) {
		serve()
	},
}

func serve() {
	// load config from environment variables or file
	conf, err, foundEnv := config.LoadEnv()
	if !foundEnv {
		conf, err = config.LoadFile(confFile)
	}
	if err != nil {
		log.Error(err)
		return
	}

	bs, _ := json.MarshalIndent(conf, "", "  ")
	log.Infof("current configuration: \n%s\n", bs)

	p, err := proxy.NewProxy(conf)
	if err != nil {
		log.Error(err)
		return
	}
	// trigger GC to release memory usage. (especially routing rule parsing)
	runtime.GC()

	// http server for profiling
	if conf.Profiling {
		go func() {
			http.Handle("/metrics", promhttp.Handler())
			log.Error(http.ListenAndServe("localhost:6060", nil))
		}()
	}

	// clean up
	go func() {
		terminate := make(chan os.Signal, 1)
		signal.Notify(terminate, syscall.SIGTERM, os.Interrupt)
		<-terminate
		p.Close()
	}()

	log.Infof("starting ...")
	p.Serve()
	log.Infof("closed")
}

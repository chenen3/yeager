package main

import (
	"encoding/json"
	_ "expvar"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/chenen3/yeager/config"
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
		log.Printf("loading config from %s\n", confFile)
		conf, err = config.LoadFile(confFile)
	}
	if err != nil {
		zap.S().Error(err)
		return
	}
	bs, _ := json.MarshalIndent(conf, "", "  ")
	log.Printf("loaded config: \n%s\n", bs)

	var zc zap.Config
	if conf.Develop {
		zc = zap.NewDevelopmentConfig()
	} else {
		zc = zap.NewProductionConfig()
	}
	zc.EncoderConfig.EncodeTime = zapcore.RFC3339TimeEncoder
	logger, err := zc.Build()
	if err != nil {
		panic(err)
	}
	defer logger.Sync()
	undo := zap.ReplaceGlobals(logger)
	defer undo()

	p, err := proxy.NewProxy(conf)
	if err != nil {
		zap.S().Error(err)
		return
	}
	// trigger GC to release memory usage. (especially routing rule parsing)
	runtime.GC()

	// http server for profiling
	if conf.Develop {
		go func() {
			zap.S().Error(http.ListenAndServe("localhost:6060", nil))
		}()
	}

	// clean up
	go func() {
		terminate := make(chan os.Signal, 1)
		signal.Notify(terminate, syscall.SIGTERM, os.Interrupt)
		<-terminate
		if err := p.Close(); err != nil {
			zap.S().Error(err)
		}
	}()

	zap.S().Infof("yeager %s starting ...", Version)
	p.Serve()
	zap.S().Info("closed")
}

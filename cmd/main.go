package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"yeager"
	"yeager/config"
	"yeager/router"
)

var confFile = flag.String("config", "/usr/local/etc/yeager/config.json", "config file")

func main() {
	flag.Parse()

	router.RegisterAssetDir(
		"/usr/local/share/yeager",
		"config/dev", // developer only
	)

	conf, err := config.Load(*confFile)
	if err != nil {
		log.Fatalln(err)
	}
	p, err := yeager.NewProxy(conf)
	if err != nil {
		log.Fatalln(err)
	}
	// trigger GC to release memory usage. (especially routing rule parsing)
	runtime.GC()

	// http server for profiling
	go func() {
		http.Handle("/metrics", promhttp.Handler())
		log.Println(http.ListenAndServe("localhost:6060", nil))
	}()

	stopTime := 3 * time.Second
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGTERM, os.Interrupt, os.Kill)
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		sig := <-c
		log.Printf("received signal \"%v\", canceling the start context and closing in %v", sig, stopTime)
		cancel()
		time.Sleep(stopTime)
		os.Exit(0)
	}()
	log.Printf("starting ...")
	p.Start(ctx)
}

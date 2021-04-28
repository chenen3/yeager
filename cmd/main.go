package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"yeager"
	"yeager/config"
	"yeager/router"
)

var confFile = flag.String("config", "/usr/local/etc/yeager/config.json", "configuration file")

func main() {
	flag.Parse()

	router.RegisterAssetsDir(
		"config/dev", // developer test
		"/usr/local/share/yeager",
	)

	conf, err := config.Load(*confFile)
	if err != nil {
		log.Fatalln(err)
	}
	p, err := yeager.NewProxy(conf)
	if err != nil {
		log.Fatalln(err)
	}
	// parsing geoip.dat obviously raise up the memory consumption,
	// trigger GC to reduce it.
	runtime.GC()

	stopTime := 3 * time.Second
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGTERM, os.Interrupt, os.Kill)
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		sig := <-c
		log.Printf("Received signal \"%v\", canceling the start context and closing in %v", sig, stopTime)
		cancel()
		time.Sleep(stopTime)
		os.Exit(0)
	}()
	p.Start(ctx)
}

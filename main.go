package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"yeager/config"
	"yeager/router"
)

var confFile = flag.String("config", "/usr/local/etc/yeager/config.json", "configuration file")

func main() {
	flag.Parse()

	router.RegisterAssetsDir(
		"release", // developer test
		"/usr/local/share/yeager",
	)

	conf, err := config.Load(*confFile)
	if err != nil {
		log.Fatalln(err)
	}

	p, err := NewProxy(conf)
	if err != nil {
		log.Fatalln(err)
	}

	// parsing geoip.dat obviously raise up the memory consumption,
	// trigger GC to reduce it.
	// before: 50 MB
	// after: 8 MB
	runtime.GC()

	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, syscall.SIGTERM, os.Interrupt, os.Kill)
		<-c
		p.Close()
	}()
	log.Fatalln(p.Start())
}

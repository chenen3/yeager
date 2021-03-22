package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"yeager/config"
)

var confFile = flag.String("config", "./config/dev/config.json", "configuration file")

func main() {
	flag.Parse()

	conf, err := config.Load(*confFile)
	if err != nil {
		log.Fatalln(err)
	}

	p, err := NewProxy(conf)
	if err != nil {
		log.Fatalln(err)
	}
	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, syscall.SIGTERM, os.Interrupt, os.Kill)
		<-c
		p.Close()
	}()
	log.Fatalln(p.Start())
}

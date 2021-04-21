package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"syscall"

	"github.com/opentracing/opentracing-go"
	"github.com/uber/jaeger-client-go"
	jconfig "github.com/uber/jaeger-client-go/config"
	"yeager/config"
	"yeager/router"
)

var confFile = flag.String("config", "/usr/local/etc/yeager/config.json", "configuration file")

func main() {
	flag.Parse()

	tracer, closer := initJaeger("yeager")
	defer closer.Close()
	opentracing.SetGlobalTracer(tracer)

	router.RegisterAssetsDir(
		"config/dev", // developer test
		"/usr/local/share/yeager",
	)
	conf, err := config.Load(*confFile)
	if err != nil {
		panic(fmt.Sprintf("ERROR: cannot load config: %s", err))
	}
	p, err := NewProxy(conf)
	if err != nil {
		panic(fmt.Sprintf("ERROR: cannot init proxy: %s", err))
	}
	// parsing geoip.dat obviously raise up the memory consumption,
	// trigger GC to reduce it.
	runtime.GC()

	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, syscall.SIGTERM, os.Interrupt, os.Kill)
		<-c
		p.Close()
	}()
	p.Start()
}

// initJaeger returns an instance of Jaeger Tracer that samples 100% of traces and logs all spans to stdout.
// sampling strategy provided via the environment variable, eg: JAEGER_SAMPLER_TYPE=const JAEGER_SAMPLER_PARAM=1
// for more detail, refer to https://www.jaegertracing.io/docs/1.22/sampling/#client-sampling-configuration
func initJaeger(service string) (opentracing.Tracer, io.Closer) {
	sampleType := os.Getenv("JAEGER_SAMPLER_TYPE")
	if sampleType == "" {
		sampleType = "const"
	}
	param := os.Getenv("JAEGER_SAMPLER_PARAM")
	if param == "" {
		// default no sampling, which will minimize the overhead
		param = "0"
	}
	sampleParam, err := strconv.ParseFloat(param, 64)
	if err != nil {
		panic(fmt.Sprintf("ERROR: cannot parse sampler param: %s, err: %s\n", param, err))
	}
	log.Printf("jaeger sampling strategy: type=%s param=%v\n", sampleType, sampleParam)

	cfg := &jconfig.Configuration{
		ServiceName: service,
		Sampler: &jconfig.SamplerConfig{
			Type:  sampleType,
			Param: sampleParam,
		},
	}
	tracer, closer, err := cfg.NewTracer(jconfig.Logger(jaeger.StdLogger))
	if err != nil {
		panic(fmt.Sprintf("ERROR: cannot init Jaeger: %v\n", err))
	}
	return tracer, closer
}

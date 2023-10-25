package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

var version string // set by build -ldflags

func replace(groups []string, a slog.Attr) slog.Attr {
	// use RFC3339 to format time
	if a.Key == slog.TimeKey {
		a.Value = slog.StringValue(a.Value.Time().Format(time.RFC3339))
		return a
	}
	// Remove the directory from the source's filename.
	if a.Key == slog.SourceKey {
		source := a.Value.Any().(*slog.Source)
		source.File = filepath.Base(source.File)
	}
	return a
}

func init() {
	infoLogger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		AddSource:   true,
		Level:       slog.LevelInfo,
		ReplaceAttr: replace,
	}))
	slog.SetDefault(infoLogger)
}

func checkIP() (string, error) {
	resp, err := http.Get("https://checkip.amazonaws.com")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	ip, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(ip)), nil
}

func genConfig(host, cliConfOutput, srvConfOutput string) error {
	if _, err := os.Stat(srvConfOutput); err == nil {
		return fmt.Errorf("file %s already exists, operation aborted", srvConfOutput)
	}
	if _, err := os.Stat(cliConfOutput); err == nil {
		return fmt.Errorf("file %s already exists, operation aborted", cliConfOutput)
	}

	cliConf, srvConf, err := GenerateConfig(host)
	if err != nil {
		return fmt.Errorf("failed to generate config: %s", err)
	}
	port := 57175
	srvConf.Listen[0].Address = fmt.Sprintf("0.0.0.0:%d", port)
	bs, err := json.MarshalIndent(srvConf, "", "\t")
	if err != nil {
		return fmt.Errorf("failed to marshal server config: %s", err)
	}
	if err = os.WriteFile(srvConfOutput, bs, 0644); err != nil {
		return fmt.Errorf("failed to write server config: %s", err)
	}
	fmt.Println("generated", srvConfOutput)

	cliConf.Proxy.Address = fmt.Sprintf("%s:%d", host, port)
	cliConf.ListenSOCKS = "127.0.0.1:1080"
	cliConf.ListenHTTP = "127.0.0.1:8080"
	bs, err = json.MarshalIndent(cliConf, "", "\t")
	if err != nil {
		return fmt.Errorf("failed to marshal client config: %s", err)
	}
	err = os.WriteFile(cliConfOutput, bs, 0644)
	if err != nil {
		return fmt.Errorf("failed to write client config: %s", err)
	}
	fmt.Println("generated", cliConfOutput)
	return nil
}

func main() {
	var flags struct {
		configFile string
		version    bool
		genConfig  bool
		ip         string
		debug      bool
	}
	flag.StringVar(&flags.configFile, "config", "", "path to configuration file")
	flag.BoolVar(&flags.version, "version", false, "print version")
	flag.BoolVar(&flags.genConfig, "genconf", false, "generate a pair of config: client.json and server.json")
	flag.StringVar(&flags.ip, "ip", "", "IP for the certificate, used with option -genconf")
	flag.BoolVar(&flags.debug, "debug", false, "enable debug logging")
	flag.Parse()

	if flags.version {
		fmt.Printf("yeager version %s\n", version)
		return
	}

	if flags.genConfig {
		ip := flags.ip
		if ip == "" {
			i, err := checkIP()
			if err != nil {
				fmt.Printf("get public IP: %s\n", err)
				return
			}
			ip = i
		}
		if err := genConfig(ip, "client.json", "server.json"); err != nil {
			fmt.Println(err)
			return
		}
		return
	}

	if flags.debug {
		debugLogger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
			AddSource:   true,
			Level:       slog.LevelDebug,
			ReplaceAttr: replace,
		}))
		slog.SetDefault(debugLogger)

		debugSrv := http.Server{Addr: "localhost:6060"}
		defer debugSrv.Close()
		go func() {
			if err := debugSrv.ListenAndServe(); err != http.ErrServerClosed {
				slog.Warn("debug server: " + err.Error())
			}
		}()
	}

	if flags.configFile == "" {
		flag.Usage()
		return
	}
	bs, err := os.ReadFile(flags.configFile)
	if err != nil {
		slog.Error("read config file: " + err.Error())
		return
	}
	var conf Config
	if err = json.Unmarshal(bs, &conf); err != nil {
		slog.Error("load config: " + err.Error())
		return
	}
	slog.Info("yeager starting", "version", version)
	services, err := StartServices(conf)
	if err != nil {
		slog.Error("start services: " + err.Error())
		closeAll(services)
		return
	}

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGTERM, syscall.SIGINT)
	sig := <-ch
	slog.Info("signal " + sig.String())
	closeAll(services)
	slog.Info("goodbye")
}

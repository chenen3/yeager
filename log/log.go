package log

import (
	"fmt"
	"log"
	"os"
)

type logger struct {
	Printf func(format string, a ...interface{}) // similar to Infof, without file information output
	Infof  func(format string, a ...interface{})
	Warnf  func(format string, a ...interface{})
	Errorf func(format string, a ...interface{})
	Error  func(a ...interface{})
}

var l *logger

// L return a global logger
func L() *logger {
	return l
}

func init() {
	l = new(logger)
	printLogger := log.New(os.Stderr, "INFO  ", log.LstdFlags)
	l.Printf = func(format string, a ...interface{}) {
		_ = printLogger.Output(2, fmt.Sprintf(format, a...))
	}
	l.Infof = log.New(os.Stderr, "INFO  ", log.LstdFlags|log.Lshortfile).Printf
	l.Warnf = log.New(os.Stderr, "WARN  ", log.LstdFlags|log.Lshortfile).Printf
	errLogger := log.New(os.Stderr, "ERROR ", log.LstdFlags|log.Lshortfile)
	l.Errorf = errLogger.Printf
	l.Error = errLogger.Print
}

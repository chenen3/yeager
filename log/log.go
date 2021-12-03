package log

import (
	"log"
	"os"
)

// logger does not implement log level
type logger struct {
	Errorf func(format string, a ...interface{})
	Error  func(a ...interface{})
	Warnf  func(format string, a ...interface{})
	Infof  func(format string, a ...interface{})
}

var l *logger

func init() {
	l = new(logger)
	errLogger := log.New(os.Stderr, "ERROR ", log.LstdFlags|log.Lshortfile)
	l.Errorf = errLogger.Printf
	l.Error = errLogger.Print
	l.Warnf = log.New(os.Stderr, "WARN ", log.LstdFlags|log.Lshortfile).Printf
	l.Infof = log.New(os.Stderr, "INFO ", log.LstdFlags|log.Lshortfile).Printf
}

// L return a global logger
func L() *logger {
	return l
}

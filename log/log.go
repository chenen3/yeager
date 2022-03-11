package log

import (
	"fmt"
	"log"
	"os"
)

var (
	infologger = log.New(os.Stderr, "INFO  ", log.LstdFlags|log.Lshortfile)
	errLogger  = log.New(os.Stderr, "ERROR ", log.LstdFlags|log.Lshortfile)
)

func Infof(format string, a ...interface{}) {
	infologger.Output(2, fmt.Sprintf(format, a...))
}

func Error(a ...interface{}) {
	errLogger.Output(2, fmt.Sprint(a...))
}
func Errorf(format string, a ...interface{}) {
	errLogger.Output(2, fmt.Sprintf(format, a...))
}

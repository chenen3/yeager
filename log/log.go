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

func Infof(msg string, a ...interface{}) {
	infologger.Output(2, fmt.Sprintf(msg, a...))
}

func Errorf(msg string, a ...interface{}) {
	errLogger.Output(2, fmt.Sprintf(msg, a...))
}

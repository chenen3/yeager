package log

import (
	"fmt"
	"log"
)

var debug bool

// Enable debug logging
func EnableDebug() {
	debug = true
}

// Debugf behaves like log.Printf if debug is enabled, otherwise does nothing
func Debugf(format string, v ...any) {
	if debug {
		log.Output(2, fmt.Sprintf(format, v...))
	}
}

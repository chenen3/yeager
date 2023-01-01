package debug

import (
	"fmt"
	"log"
)

var debug bool

// Enable debug logging
func Enable() {
	debug = true
}

// Logf behaves like log.Printf if debug is enabled, otherwise does nothing
func Logf(format string, v ...any) {
	if debug {
		log.Output(2, fmt.Sprintf(format, v...))
	}
}

package debug

import (
	"fmt"
	"log"
)

var enable bool

// Enable debug logging
func Enable() {
	enable = true
}

// Logf is equivalent to log.Printf, if debug is enabled
func Logf(format string, v ...any) {
	if !enable {
		return
	}
	log.Default().Output(2, fmt.Sprintf(format, v...))
}

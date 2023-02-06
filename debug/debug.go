package debug

import (
	"fmt"
	"log"
	"strconv"
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

// Counter implements the expvar.Var interface
// and collects result from the internal counter function
type Counter []func() int

// Register registers counter function
func (c *Counter) Register(f func() int) {
	*c = append(*c, f)
}

func (c *Counter) String() string {
	var total int
	for _, f := range *c {
		total += f()
	}
	return strconv.FormatInt(int64(total), 10)
}

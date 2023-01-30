package debug

import "strconv"

var enable bool

// Enable debug logging
func Enable() {
	enable = true
}

// Enabled returns true if debugging is enabled
func Enabled() bool {
	return enable
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

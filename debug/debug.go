package debug

var enable bool

// Enable debug logging
func Enable() {
	enable = true
}

// Enabled returns whether debugging is enabled
func Enabled() bool {
	return enable
}

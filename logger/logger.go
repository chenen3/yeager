package logger

import (
	"io"
	"log"
	"os"
)

var (
	// Debug logger does not output anything until a real output destination is set
	Debug = log.New(io.Discard, "[DEBUG] ", log.LstdFlags|log.Lshortfile)
	Info  = log.New(os.Stderr, "[INFO] ", log.LstdFlags|log.Lshortfile)
	Error = log.New(os.Stderr, "[ERROR] ", log.LstdFlags|log.Lshortfile)
)

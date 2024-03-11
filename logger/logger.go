package logger

import (
	"io"
	"log"
	"os"
)

var (
	// Debug does nothing until a valid output destination is set
	Debug = log.New(io.Discard, "[DEBUG] ", log.LstdFlags|log.Lshortfile)
	Info  = log.New(os.Stderr, "[INFO] ", log.LstdFlags|log.Lshortfile)
	Error = log.New(os.Stderr, "[ERROR] ", log.LstdFlags|log.Lshortfile)
)

package logger

import (
	"log"
	"os"
)

var (
	Info  = log.New(os.Stderr, "[INFO] ", log.LstdFlags|log.Lshortfile)
	Error = log.New(os.Stderr, "[ERROR] ", log.LstdFlags|log.Lshortfile)
)

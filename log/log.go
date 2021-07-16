package log

import (
	"fmt"
	"log"
	"os"
)

var logger = log.New(os.Stderr, "", log.LstdFlags|log.Lshortfile)

func Error(v ...interface{}) {
	nv := []interface{}{"ERROR: "}
	nv = append(nv, v...)
	logger.Output(2, fmt.Sprint(nv...))
}

func Errorf(format string, v ...interface{}) {
	format = "ERROR: " + format
	logger.Output(2, fmt.Sprintf(format, v...))
}

func Warn(v ...interface{}) {
	nv := []interface{}{"WARN: "}
	nv = append(nv, v...)
	logger.Output(2, fmt.Sprint(nv...))
}

func Warnf(format string, v ...interface{}) {
	format = "WARN: " + format
	logger.Output(2, fmt.Sprintf(format, v...))
}

func Infof(format string, v ...interface{}) {
	logger.Output(2, fmt.Sprintf(format, v...))
}

func Debugf(format string, v ...interface{}) {
	format = "DEBUG: " + format
	logger.Output(2, fmt.Sprintf(format, v...))
}

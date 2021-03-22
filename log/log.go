package log

import (
	"fmt"
	"log"
	"os"
)

var std = log.New(os.Stderr, "", log.LstdFlags|log.Llongfile)

func Error(v ...interface{}) {
	nv := []interface{}{"ERROR: "}
	nv = append(nv, v...)
	std.Output(2, fmt.Sprint(nv...))
}

func Errorf(format string, v ...interface{}) {
	format = "ERROR: " + format
	std.Output(2, fmt.Sprintf(format, v...))
}

func Infof(format string, v ...interface{}) {
	std.Output(2, fmt.Sprintf(format, v...))
}

func Debugf(format string, v ...interface{}) {
	format = "DEBUG: " + format
	std.Output(2, fmt.Sprintf(format, v...))
}

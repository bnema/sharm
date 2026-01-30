package logger

import (
	"log"
	"os"
)

var (
	Info  *log.Logger
	Error *log.Logger
	Debug *log.Logger
	Warn  *log.Logger
)

func init() {
	logFlags := log.Ldate | log.Ltime | log.LUTC | log.Lshortfile

	Info = log.New(os.Stdout, "INFO: ", logFlags)
	Error = log.New(os.Stdout, "ERROR: ", logFlags)
	Debug = log.New(os.Stdout, "DEBUG: ", logFlags)
	Warn = log.New(os.Stdout, "WARN: ", logFlags)
}

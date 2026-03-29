package logger

import (
	"log"
	"os"

	"github.com/bnema/sharm/internal/port"
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

var _ port.Logger = (*StdLogger)(nil)

// StdLogger wraps the package-level loggers as a port.Logger implementation.
type StdLogger struct{}

func NewStdLogger() *StdLogger { return &StdLogger{} }

func (l *StdLogger) Infof(format string, args ...any)  { Info.Printf(format, args...) }
func (l *StdLogger) Errorf(format string, args ...any) { Error.Printf(format, args...) }
func (l *StdLogger) Debugf(format string, args ...any) { Debug.Printf(format, args...) }
func (l *StdLogger) Warnf(format string, args ...any)  { Warn.Printf(format, args...) }

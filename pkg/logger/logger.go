package logger

import (
	"log"
	"os"
)

type Logger interface {
	Info(msg string, args ...interface{})
	Infof(format string, args ...interface{})
	Warn(msg string, args ...interface{})
	Warnf(format string, args ...interface{})
	Error(msg string, args ...interface{})
	Errorf(format string, args ...interface{})
}

type DefaultLogger struct {
	l *log.Logger
}

func New() Logger {
	return &DefaultLogger{
		l: log.New(os.Stdout, "[WORKFLOW] ", log.LstdFlags|log.Lmsgprefix),
	}
}

func (d *DefaultLogger) Info(msg string, args ...interface{}) {
	d.l.Printf("INFO: "+msg, args...)
}
func (d *DefaultLogger) Infof(format string, args ...interface{}) {
	d.l.Printf("INFO: "+format, args...)
}
func (d *DefaultLogger) Warn(msg string, args ...interface{}) {
	d.l.Printf("WARN: "+msg, args...)
}
func (d *DefaultLogger) Warnf(format string, args ...interface{}) {
	d.l.Printf("WARN: "+format, args...)
}
func (d *DefaultLogger) Error(msg string, args ...interface{}) {
	d.l.Printf("ERROR: "+msg, args...)
}
func (d *DefaultLogger) Errorf(format string, args ...interface{}) {
	d.l.Printf("ERROR: "+format, args...)
}

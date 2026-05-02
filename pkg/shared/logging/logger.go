package logging

import (
	"fmt"
	"log"
	"os"
	"sync"
	"time"
)

type LogSink interface {
	Ship(entry LogEntry)
}

type Logger struct {
	NodeID    string
	Component string
	Sink      LogSink
	mu        sync.Mutex
}

func NewLogger(nodeID, component string) *Logger {
	return &Logger{
		NodeID:    nodeID,
		Component: component,
	}
}

func (l *Logger) SetSink(s LogSink) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.Sink = s
}

func (l *Logger) Log(level LogLevel, msg string) {
	entry := LogEntry{
		Timestamp: time.Now(),
		NodeID:    l.NodeID,
		Level:     level,
		Component: l.Component,
		Message:   msg,
	}

	// Local print
	_, err := fmt.Fprintf(os.Stderr, "[%s] %s [%s] %s: %s\n",
		entry.Timestamp.Format("2006-01-02 15:04:05"),
		entry.Level,
		entry.NodeID,
		entry.Component,
		entry.Message)
	if err != nil {
		panic(fmt.Sprintf("Failed to write log: %v", err))
	}

	// Ship to sink
	l.mu.Lock()
	sink := l.Sink
	l.mu.Unlock()

	if sink != nil {
		sink.Ship(entry)
	}
}

func (l *Logger) Infof(format string, v ...interface{}) {
	l.Log(LevelInfo, fmt.Sprintf(format, v...))
}

func (l *Logger) Warnf(format string, v ...interface{}) {
	l.Log(LevelWarn, fmt.Sprintf(format, v...))
}

func (l *Logger) Errorf(format string, v ...interface{}) {
	l.Log(LevelError, fmt.Sprintf(format, v...))
}

func (l *Logger) Debugf(format string, v ...interface{}) {
	l.Log(LevelDebug, fmt.Sprintf(format, v...))
}

// Global logger for convenience (initialized in main)
var Global *Logger = NewLogger("default", "default")

func InitGlobal(nodeID, component string) {
	Global = NewLogger(nodeID, component)
	// Optionally redirect standard log output
	log.SetFlags(0)
	log.SetOutput(lwriter{l: Global})
}

type lwriter struct {
	l *Logger
}

func (w lwriter) Write(p []byte) (n int, err error) {
	w.l.Log(LevelInfo, string(p))
	return len(p), nil
}

package level

import (
	"fmt"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
)

var logLevel = LevelInfo

// A Level is a logging priority. Higher levels are more important.
type Level int

const (
	// LevelDebug logs are typically voluminous, and are usually disabled in
	// production.
	LevelDebug Level = iota
	// LevelInfo is the default logging priority.
	LevelInfo
	// LevelWarn logs are more important than Info, but don't need individual
	// human review.
	LevelWarn
	// LevelError logs are high-priority. If an application is running smoothly,
	// it shouldn't generate any error-level logs.
	LevelError
)

var emptyLogger = &EmptyLogger{}

type EmptyLogger struct{}

func (l *EmptyLogger) Log(keyvals ...interface{}) error {
	return nil
}

// SetLogLevel sets the log level.
func SetLogLevel(level string) error {
	switch level {
	case "debug":
		logLevel = LevelDebug
	case "info":
		logLevel = LevelInfo
	case "warn":
		logLevel = LevelWarn
	case "error":
		logLevel = LevelError
	default:
		return fmt.Errorf("unrecognized log level %s", level)
	}
	return nil
}

// Error returns a logger that includes a Key/ErrorValue pair.
func Error(logger log.Logger) log.Logger {
	if logLevel <= LevelError {
		return level.Error(logger)
	}
	return emptyLogger
}

// Warn returns a logger that includes a Key/WarnValue pair.
func Warn(logger log.Logger) log.Logger {
	if logLevel <= LevelWarn {
		return level.Warn(logger)
	}
	return emptyLogger
}

// Info returns a logger that includes a Key/InfoValue pair.
func Info(logger log.Logger) log.Logger {
	if logLevel <= LevelInfo {
		return level.Info(logger)
	}
	return emptyLogger
}

// Debug returns a logger that includes a Key/DebugValue pair.
func Debug(logger log.Logger) log.Logger {
	if logLevel <= LevelDebug {
		return level.Debug(logger)
	}
	return emptyLogger
}

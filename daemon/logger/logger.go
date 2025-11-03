// Package logger provides structured logging functionality with color-coded output and log rotation.
package logger

import (
	"fmt"
	"log"
)

// LogLevel represents the logging verbosity level
type LogLevel int

const (
	// LevelDebug enables all logging including debug messages
	LevelDebug LogLevel = iota
	// LevelInfo enables info, warning, and error messages
	LevelInfo
	// LevelWarning enables warning and error messages only
	LevelWarning
	// LevelError enables error messages only
	LevelError
)

var currentLevel = LevelWarning // Default to WARNING level for production

// Color codes for terminal output
const (
	ColorReset  = "\033[0m"
	ColorRed    = "\033[31m"
	ColorGreen  = "\033[32m"
	ColorYellow = "\033[33m"
	ColorBlue   = "\033[34m"
	ColorPurple = "\033[35m"
	ColorCyan   = "\033[36m"
	ColorWhite  = "\033[37m"
)

// SetLevel sets the global logging level
func SetLevel(level LogLevel) {
	currentLevel = level
}

// GetLevel returns the current logging level
func GetLevel() LogLevel {
	return currentLevel
}

// Info logs informational messages in blue
func Info(format string, v ...interface{}) {
	if currentLevel <= LevelInfo {
		log.Printf(ColorBlue+format+ColorReset, v...)
	}
}

// Success logs success messages in green
func Success(format string, v ...interface{}) {
	if currentLevel <= LevelInfo {
		log.Printf(ColorGreen+format+ColorReset, v...)
	}
}

// Warning logs warning messages in yellow
func Warning(format string, v ...interface{}) {
	if currentLevel <= LevelWarning {
		log.Printf(ColorYellow+"WARNING: "+format+ColorReset, v...)
	}
}

// Error logs error messages in red
func Error(format string, v ...interface{}) {
	if currentLevel <= LevelError {
		log.Printf(ColorRed+"ERROR: "+format+ColorReset, v...)
	}
}

// Debug logs debug messages in cyan (only if debug level is enabled)
func Debug(format string, v ...interface{}) {
	if currentLevel <= LevelDebug {
		log.Printf(ColorCyan+"DEBUG: "+format+ColorReset, v...)
	}
}

// Fatal logs fatal error and exits
func Fatal(format string, v ...interface{}) {
	log.Fatalf(ColorRed+"FATAL: "+format+ColorReset, v...)
}

// Plain logs without color
func Plain(format string, v ...interface{}) {
	log.Printf(format, v...)
}

// Blue alias for Info
func Blue(format string, v ...interface{}) {
	Info(format, v...)
}

// Yellow alias for Warning
func Yellow(format string, v ...interface{}) {
	Warning(format, v...)
}

// Green alias for Success
func Green(format string, v ...interface{}) {
	Success(format, v...)
}

// LightGreen logs in light green
func LightGreen(format string, v ...interface{}) {
	if currentLevel <= LevelInfo {
		log.Printf("\033[92m"+format+ColorReset, v...)
	}
}

// Printf is a wrapper for standard log.Printf
func Printf(format string, v ...interface{}) {
	if currentLevel <= LevelInfo {
		log.Printf(format, v...)
	}
}

// Println is a wrapper for standard log.Println
func Println(v ...interface{}) {
	if currentLevel <= LevelInfo {
		log.Println(v...)
	}
}

// Sprintf formats and returns a string
func Sprintf(format string, v ...interface{}) string {
	return fmt.Sprintf(format, v...)
}

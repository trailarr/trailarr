package internal

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var (
	logWriter     *os.File
	logWriterOnce sync.Once
	logFileBase   string
)

// Call this once at startup to set up the log file writer and reset log counter
func InitTrailarrLogWriter(logPath string) {
	logWriterOnce.Do(func() {
		logFileBase = logPath
		openLogFile()
	})
}

func openLogFile() {
	if logWriter != nil {
		logWriter.Close()
	}
	// Always open trailarr.txt for writing
	f, err := os.OpenFile(logFileBase, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err == nil {
		logWriter = f
	}
}

// LogLevel struct for log levels
type LogLevel struct {
	Name  string
	Value int
}

var (
	DEBUG = LogLevel{"Debug", 1}
	INFO  = LogLevel{"Info", 2}
	WARN  = LogLevel{"Warn", 3}
	ERROR = LogLevel{"Error", 4}
	FATAL = LogLevel{"Fatal", 5}
)

func TrailarrLog(level LogLevel, component, message string, args ...interface{}) {
	if !ShouldLog(level) {
		return
	}
	msg := fmt.Sprintf(message, args...)
	// Format timestamp: yyyy-mm-dd HH:MM:SS.s (1 decimal ms)
	now := time.Now()
	timestamp := now.Format("2006-01-02 15:04:05")
	ms := now.Nanosecond() / 1e8 // tenths of a second
	logLine := fmt.Sprintf("%s.%d|%s|%s|%s\n", timestamp, ms, level.Name, component, msg)
	fmt.Fprint(os.Stdout, logLine)
	if logWriter != nil {
		logWriter.Write([]byte(logLine))
		// Check file size and rotate if needed
		logWriter.Sync() // flush to disk
		fi, err := os.Stat(logFileBase)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[TrailarrLog] Stat error: %v\n", err)
		} else if fi.Size() > 1024*1024 { // 1MB
			rotateLogFile()
		}
	}
}

func rotateLogFile() {
	logWriter.Close()
	ext := filepath.Ext(logFileBase)
	base := logFileBase[:len(logFileBase)-len(ext)]
	// Find all existing rotated log files and renumber them
	pattern := fmt.Sprintf("%s-*.txt", base)
	files, _ := filepath.Glob(pattern)
	// Sort descending by number
	type logFile struct {
		path string
		num  int
	}
	var logFiles []logFile
	for _, f := range files {
		var n int
		fmt.Sscanf(f, base+"-%d.txt", &n)
		if n > 0 {
			logFiles = append(logFiles, logFile{f, n})
		}
	}
	// Renumber from highest to lowest
	for i := len(logFiles) - 1; i >= 0; i-- {
		lf := logFiles[i]
		newName := fmt.Sprintf("%s-%d.txt", base, lf.num+1)
		os.Rename(lf.path, newName)
	}
	// Rename trailarr.txt to trailarr-1.txt
	os.Rename(logFileBase, fmt.Sprintf("%s-1.txt", base))
	openLogFile()
	if logWriter == nil {
		fmt.Fprintf(os.Stderr, "[TrailarrLog] Failed to open new trailarr.txt after rotation\n")
	}
}

// Helper to get log level from config
func GetLogLevel() LogLevel {
	config, err := readConfigFile()
	if err != nil {
		return DEBUG
	}
	if general, ok := config["general"].(map[string]interface{}); ok {
		if v, ok := general["logLevel"].(string); ok {
			switch v {
			case "Debug":
				return DEBUG
			case "Info":
				return INFO
			case "Warn":
				return WARN
			case "Error":
				return ERROR
			}
		}
	}
	return DEBUG
}

// ShouldLog returns true if the message should be logged at the given level
func ShouldLog(level LogLevel) bool {
	cur := GetLogLevel().Value
	return level.Value >= cur
}

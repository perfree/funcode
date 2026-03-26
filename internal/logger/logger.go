package logger

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// L is the global logger instance.
var L *slog.Logger

// logFile keeps a reference for cleanup.
var logFile *os.File

func init() {
	// Default: discard all logs until Init() is called.
	L = slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError + 1}))
}

// Init initializes the global logger.
// level: "debug", "info", "warn", "error" (case-insensitive, default "info").
// logDir: directory for log files (default "logs", relative to cwd).
func Init(level, logDir string) error {
	slogLevel := parseLevel(level)

	if logDir == "" {
		logDir = "logs"
	}

	if err := os.MkdirAll(logDir, 0755); err != nil {
		return fmt.Errorf("creating log dir %s: %w", logDir, err)
	}

	logPath := filepath.Join(logDir, "funcode.log")
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("opening log file %s: %w", logPath, err)
	}

	logFile = f
	L = slog.New(slog.NewJSONHandler(f, &slog.HandlerOptions{Level: slogLevel}))
	return nil
}

// Close closes the log file. Call this on shutdown.
func Close() {
	if logFile != nil {
		_ = logFile.Close()
	}
}

func parseLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// Convenience functions

func Debug(msg string, args ...any) { L.Debug(msg, args...) }
func Info(msg string, args ...any)  { L.Info(msg, args...) }
func Warn(msg string, args ...any)  { L.Warn(msg, args...) }
func Error(msg string, args ...any) { L.Error(msg, args...) }

package gatewaydebug

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
)

const (
	EnvName         = "SUB2API_DEBUG_GATEWAY_BODY"
	defaultFilename = "gateway_debug.log"
)

type Logger struct {
	writer   io.Writer
	mu       sync.Mutex
	sequence atomic.Uint64
}

var (
	defaultOnce   sync.Once
	defaultLogger *Logger
)

func Default() *Logger {
	defaultOnce.Do(func() {
		path := strings.TrimSpace(os.Getenv(EnvName))
		if path == "" {
			return
		}
		if isTruthy(path) {
			path = defaultFilename
		}
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			path = filepath.Join(path, defaultFilename)
		}
		if dir := filepath.Dir(path); dir != "." {
			if err := os.MkdirAll(dir, 0755); err != nil {
				slog.Error("failed to create gateway debug log directory", "dir", dir, "error", err)
				return
			}
		}
		file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			slog.Error("failed to open gateway debug log file", "path", path, "error", err)
			return
		}
		defaultLogger = New(file)
		slog.Info("gateway debug logging enabled", "path", path)
	})
	return defaultLogger
}

func New(writer io.Writer) *Logger {
	if writer == nil {
		return nil
	}
	return &Logger{writer: writer}
}

func (l *Logger) NextID() uint64 {
	if l == nil {
		return 0
	}
	return l.sequence.Add(1)
}

func (l *Logger) Write(write func(io.Writer)) {
	if l == nil || l.writer == nil || write == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	write(l.writer)
}

func isTruthy(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

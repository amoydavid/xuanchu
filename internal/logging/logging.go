package logging

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
)

type LogConfig struct {
	Level  string      `toml:"level"`
	Format string      `toml:"format"`
	File   *FileConfig `toml:"file"`
}

type FileConfig struct {
	Path       string `toml:"path"`
	Rotate     string `toml:"rotate"`
	MaxSizeMB  int    `toml:"max_size_mb"`
	MaxAgeDays int    `toml:"max_age_days"`
}

type Logger struct {
	inner *slog.Logger
	cfg   LogConfig
}

func (l *Logger) Debug(msg string, args ...any) { l.inner.Debug(msg, args...) }
func (l *Logger) Info(msg string, args ...any)  { l.inner.Info(msg, args...) }
func (l *Logger) Warn(msg string, args ...any)  { l.inner.Warn(msg, args...) }
func (l *Logger) Error(msg string, args ...any) { l.inner.Error(msg, args...) }

func (l *Logger) With(args ...any) *Logger {
	return &Logger{inner: l.inner.With(args...), cfg: l.cfg}
}

func Setup(cfg LogConfig, stderr io.Writer) (*Logger, func() error, error) {
	level := parseLevel(cfg.Level)
	opts := &slog.HandlerOptions{Level: level}

	var writers []io.Writer
	if stderr != nil {
		writers = append(writers, stderr)
	}

	var closeFn func() error

	if cfg.File != nil && cfg.File.Path != "" {
		rw, cleanup, err := newRotateWriter(cfg.File)
		if err != nil {
			return nil, nil, fmt.Errorf("logging: %w", err)
		}
		closeFn = cleanup
		writers = append(writers, rw)
	}

	if len(writers) == 0 {
		writers = append(writers, io.Discard)
	}

	var handler slog.Handler
	w := io.MultiWriter(writers...)
	switch cfg.Format {
	case "json":
		handler = slog.NewJSONHandler(w, opts)
	default:
		handler = slog.NewTextHandler(w, opts)
	}

	logger := &Logger{inner: slog.New(handler), cfg: cfg}

	if closeFn == nil {
		closeFn = func() error { return nil }
	}

	return logger, closeFn, nil
}

func parseLevel(s string) slog.Level {
	switch s {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func expandPath(path string) string {
	if path == "" {
		return ""
	}
	if path[0] == '~' {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[1:])
	}
	return path
}

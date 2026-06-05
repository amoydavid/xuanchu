package logging

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetup_ZeroConfig(t *testing.T) {
	logger, closeFn, err := Setup(LogConfig{}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if logger == nil {
		t.Fatal("logger is nil")
	}
	if err := closeFn(); err != nil {
		t.Fatalf("closeFn returned error: %v", err)
	}
}

func TestSetup_WithFile(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "test.log")

	cfg := LogConfig{
		Level: "debug",
		File: &FileConfig{
			Path: logPath,
		},
	}

	logger, closeFn, err := Setup(cfg, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	logger.Info("hello file", "key", "value")

	if err := closeFn(); err != nil {
		t.Fatalf("closeFn returned error: %v", err)
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}

	if len(data) == 0 {
		t.Fatal("log file is empty")
	}
	if !strings.Contains(string(data), "hello file") {
		t.Fatalf("log file does not contain message; got: %s", string(data))
	}
}

func TestSetup_WithFile_ExpandsTilde(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home dir")
	}

	dir := filepath.Join(home, ".xuanchu-test-tmp", strings.TrimPrefix(t.Name(), "Test"))
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	defer os.RemoveAll(dir)

	logPath := "~/" + strings.TrimPrefix(dir, home+"/") + "/test.log"

	cfg := LogConfig{
		Level: "info",
		File: &FileConfig{
			Path: logPath,
		},
	}

	logger, closeFn, err := Setup(cfg, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	logger.Info("tilde test")
	if err := closeFn(); err != nil {
		t.Fatalf("closeFn returned error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "test.log"))
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}
	if !strings.Contains(string(data), "tilde test") {
		t.Fatalf("log file does not contain message; got: %s", string(data))
	}
}

func TestSetup_InvalidLevel_DefaultsToInfo(t *testing.T) {
	var buf bytes.Buffer
	cfg := LogConfig{Level: "nonsense"}

	logger, closeFn, err := Setup(cfg, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer closeFn()

	logger.Debug("should not appear")
	logger.Info("should appear")

	output := buf.String()
	if strings.Contains(output, "should not appear") {
		t.Fatal("debug message should not appear at info level")
	}
	if !strings.Contains(output, "should appear") {
		t.Fatal("info message should appear")
	}
}

func TestSetup_JSONFormat(t *testing.T) {
	var buf bytes.Buffer
	cfg := LogConfig{Format: "json", Level: "info"}

	logger, closeFn, err := Setup(cfg, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer closeFn()

	logger.Info("json test", "key", "value")

	line := strings.TrimSpace(buf.String())
	var parsed map[string]any
	if err := json.Unmarshal([]byte(line), &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v\nraw: %s", err, line)
	}
	if parsed["msg"] != "json test" {
		t.Fatalf("unexpected msg: %v", parsed["msg"])
	}
}

func TestSetup_WithStderr(t *testing.T) {
	var buf bytes.Buffer
	cfg := LogConfig{Level: "info"}

	logger, closeFn, err := Setup(cfg, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer closeFn()

	logger.Info("stderr test")

	output := buf.String()
	if !strings.Contains(output, "stderr test") {
		t.Fatal("stderr did not receive log output")
	}
}

func TestExpandPath_Empty(t *testing.T) {
	if got := expandPath(""); got != "" {
		t.Fatalf("expected empty string, got %q", got)
	}
}

func TestExpandPath_Tilde(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home dir")
	}

	got := expandPath("~/foo/bar")
	want := filepath.Join(home, "foo/bar")
	if got != want {
		t.Fatalf("expandPath(\"~/foo/bar\") = %q, want %q", got, want)
	}
}

func TestExpandPath_AbsolutePath(t *testing.T) {
	got := expandPath("/tmp/test.log")
	if got != "/tmp/test.log" {
		t.Fatalf("expected /tmp/test.log, got %q", got)
	}
}

func TestWith_PropagatesFields(t *testing.T) {
	var buf bytes.Buffer
	cfg := LogConfig{Level: "debug"}

	logger, closeFn, err := Setup(cfg, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer closeFn()

	child := logger.With("request_id", "abc123")
	child.Info("child message")

	output := buf.String()
	if !strings.Contains(output, "abc123") {
		t.Fatal("child logger did not propagate fields")
	}
	if !strings.Contains(output, "child message") {
		t.Fatal("child logger did not log message")
	}
}

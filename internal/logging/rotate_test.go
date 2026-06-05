package logging

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRotateNone(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "test.log")

	w, cleanup, err := newRotateWriter(&FileConfig{
		Path:   logPath,
		Rotate: "none",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	msg := []byte("hello rotate none\n")
	if _, err := w.Write(msg); err != nil {
		t.Fatalf("write error: %v", err)
	}

	if err := cleanup(); err != nil {
		t.Fatalf("close error: %v", err)
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read error: %v", err)
	}

	if !strings.Contains(string(data), "hello rotate none") {
		t.Fatalf("expected content not found; got: %s", string(data))
	}
}

func TestRotateDaily(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "app.log")

	w, cleanup, err := newRotateWriter(&FileConfig{
		Path:   logPath,
		Rotate: "daily",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer cleanup()

	w.mu.Lock()
	w.curDate = "2000-01-01"
	w.mu.Unlock()

	msg := []byte("rotated message\n")
	if _, err := w.Write(msg); err != nil {
		t.Fatalf("write error: %v", err)
	}

	today := time.Now().Format("2006-01-02")
	ext := filepath.Ext(filepath.Base(logPath))
	stem := strings.TrimSuffix(filepath.Base(logPath), ext)
	newPath := filepath.Join(dir, stem+"-"+today+ext)

	data, err := os.ReadFile(newPath)
	if err != nil {
		t.Fatalf("read new date file error: %v", err)
	}

	if !strings.Contains(string(data), "rotated message") {
		t.Fatalf("expected content not found; got: %s", string(data))
	}
}

func TestRotateSize(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "app.log")

	w, cleanup, err := newRotateWriter(&FileConfig{
		Path:   logPath,
		Rotate: "size",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer cleanup()

	w.mu.Lock()
	w.maxSize = 50
	w.mu.Unlock()

	first := []byte(strings.Repeat("a", 50))
	if _, err := w.Write(first); err != nil {
		t.Fatalf("first write error: %v", err)
	}

	second := []byte("b")
	if _, err := w.Write(second); err != nil {
		t.Fatalf("second write error: %v", err)
	}

	backupPath := logPath + ".1"
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		t.Fatal("backup file was not created")
	}

	backupData, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatalf("read backup error: %v", err)
	}
	if len(backupData) != 50 {
		t.Fatalf("backup should contain 50 bytes, got %d", len(backupData))
	}

	currentData, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read current error: %v", err)
	}
	if !strings.Contains(string(currentData), "b") {
		t.Fatalf("current file should contain 'b'; got: %s", string(currentData))
	}
}

func TestCleanOldLogs(t *testing.T) {
	dir := t.TempDir()

	oldFile := filepath.Join(dir, "old.log")
	if err := os.WriteFile(oldFile, []byte("old"), 0644); err != nil {
		t.Fatalf("create old file: %v", err)
	}
	oldTime := time.Now().AddDate(0, 0, -10)
	if err := os.Chtimes(oldFile, oldTime, oldTime); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	newFile := filepath.Join(dir, "new.log")
	if err := os.WriteFile(newFile, []byte("new"), 0644); err != nil {
		t.Fatalf("create new file: %v", err)
	}

	w := &rotateWriter{dir: dir}
	w.cleanOldLogs(dir, 5)

	if _, err := os.Stat(oldFile); !os.IsNotExist(err) {
		t.Fatal("old file should have been deleted")
	}

	if _, err := os.Stat(newFile); os.IsNotExist(err) {
		t.Fatal("new file should still exist")
	}
}

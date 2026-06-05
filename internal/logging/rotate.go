package logging

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const maxBackups = 5

const defaultMaxSize int64 = 100 * 1024 * 1024

type rotateWriter struct {
	mu      sync.Mutex
	file    *os.File
	dir     string
	base    string
	rotate  string
	maxSize int64
	curDate string
	curSize int64
	nowFn   func() time.Time
}

func newRotateWriter(fc *FileConfig) (*rotateWriter, func() error, error) {
	path := expandPath(fc.Path)
	dir := filepath.Dir(path)
	base := filepath.Base(path)

	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, nil, err
	}

	rotate := fc.Rotate
	if rotate == "" {
		rotate = "none"
	}

	maxSize := defaultMaxSize
	if fc.MaxSizeMB > 0 {
		maxSize = int64(fc.MaxSizeMB) * 1024 * 1024
	}

	w := &rotateWriter{
		dir:     dir,
		base:    base,
		rotate:  rotate,
		maxSize: maxSize,
		nowFn:   time.Now,
	}

	if rotate == "daily" {
		w.curDate = w.nowFn().Format("2006-01-02")
	}

	if err := w.openCurrent(); err != nil {
		return nil, nil, err
	}

	if fc.MaxAgeDays > 0 {
		w.cleanOldLogs(dir, fc.MaxAgeDays)
	}

	return w, w.Close, nil
}

func (w *rotateWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	switch w.rotate {
	case "daily":
		today := w.nowFn().Format("2006-01-02")
		if today != w.curDate {
			if err := w.rotateDaily(today); err != nil {
				return 0, err
			}
		}
	case "size":
		if w.curSize+int64(len(p)) > w.maxSize {
			if err := w.rotateSize(); err != nil {
				return 0, err
			}
		}
	}

	n, err := w.file.Write(p)
	if err != nil {
		return n, err
	}
	w.curSize += int64(n)
	return n, nil
}

func (w *rotateWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file != nil {
		err := w.file.Close()
		w.file = nil
		return err
	}
	return nil
}

func (w *rotateWriter) currentPath() string {
	switch w.rotate {
	case "daily":
		ext := filepath.Ext(w.base)
		stem := strings.TrimSuffix(w.base, ext)
		return filepath.Join(w.dir, stem+"-"+w.curDate+ext)
	default:
		return filepath.Join(w.dir, w.base)
	}
}

func (w *rotateWriter) openCurrent() error {
	path := w.currentPath()
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}

	fi, err := f.Stat()
	if err != nil {
		f.Close()
		return err
	}

	w.file = f
	w.curSize = fi.Size()
	return nil
}

func (w *rotateWriter) rotateDaily(today string) error {
	if w.file != nil {
		if err := w.file.Close(); err != nil {
			return err
		}
	}
	w.curDate = today
	return w.openCurrent()
}

func (w *rotateWriter) rotateSize() error {
	if w.file != nil {
		if err := w.file.Close(); err != nil {
			return err
		}
	}

	basePath := filepath.Join(w.dir, w.base)

	os.Remove(fmt.Sprintf("%s.%d", basePath, maxBackups))

	for i := maxBackups; i >= 2; i-- {
		src := fmt.Sprintf("%s.%d", basePath, i-1)
		dst := fmt.Sprintf("%s.%d", basePath, i)
		os.Rename(src, dst)
	}

	os.Rename(basePath, fmt.Sprintf("%s.1", basePath))

	return w.openCurrent()
}

func (w *rotateWriter) cleanOldLogs(dir string, maxAgeDays int) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	cutoff := time.Now().AddDate(0, 0, -maxAgeDays)

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			os.Remove(filepath.Join(dir, entry.Name()))
		}
	}
}

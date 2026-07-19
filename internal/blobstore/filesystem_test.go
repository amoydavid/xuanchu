package blobstore

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFilesystemRoundTripAndPermissions(t *testing.T) {
	root := t.TempDir()
	store := NewFilesystem(root)
	key := "workspaces/ws/attachments/id"
	payload := []byte("abc")
	if err := store.Put(context.Background(), key, bytes.NewReader(payload), int64(len(payload)), "text/plain"); err != nil {
		t.Fatalf("Put: %v", err)
	}
	r, info, err := store.Open(context.Background(), key)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer r.Close()
	got, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("payload = %q, want %q", got, payload)
	}
	if info.Size != int64(len(payload)) {
		t.Fatalf("size = %d, want %d", info.Size, len(payload))
	}
	stat, err := os.Stat(filepath.Join(root, filepath.FromSlash(key)))
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if stat.Mode().Perm() != 0o600 {
		t.Fatalf("file mode = %o, want 0600", stat.Mode().Perm())
	}
	dirStat, err := os.Stat(filepath.Join(root, "workspaces", "ws", "attachments"))
	if err != nil {
		t.Fatalf("Dir Stat: %v", err)
	}
	if dirStat.Mode().Perm() != 0o700 {
		t.Fatalf("dir mode = %o, want 0700", dirStat.Mode().Perm())
	}
}

func TestFilesystemDeleteAndIdempotent(t *testing.T) {
	root := t.TempDir()
	store := NewFilesystem(root)
	key := "workspaces/ws/attachments/id"
	if err := store.Put(context.Background(), key, strings.NewReader("x"), 1, "text/plain"); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(context.Background(), key); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	// 再次 Delete 应幂等。
	if err := store.Delete(context.Background(), key); err != nil {
		t.Fatalf("Delete idempotent: %v", err)
	}
}

func TestFilesystemRejectsSizeMismatch(t *testing.T) {
	root := t.TempDir()
	store := NewFilesystem(root)
	err := store.Put(context.Background(), "workspaces/ws/attachments/id", strings.NewReader("abc"), 5, "text/plain")
	if err == nil || !strings.Contains(err.Error(), "declared size") {
		t.Fatalf("err = %v", err)
	}
	// 失败后不应留下目标文件。
	if _, err := os.Stat(filepath.Join(root, "workspaces", "ws", "attachments", "id")); !os.IsNotExist(err) {
		t.Fatalf("leftover file present: %v", err)
	}
}

func TestFilesystemRejectsTraversal(t *testing.T) {
	cases := []string{
		"../escape",
		"workspaces/../../escape",
		"..",
		"",
		".tmp/evil",
		".hidden",
		"workspaces/.hidden/id",
	}
	root := t.TempDir()
	store := NewFilesystem(root)
	for _, key := range cases {
		t.Run(key, func(t *testing.T) {
			err := store.Put(context.Background(), key, strings.NewReader("x"), 1, "text/plain")
			if err == nil {
				t.Fatalf("expected rejection for %q", key)
			}
		})
	}
}

func TestFilesystemHealthCreatesRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "nested", "attachments")
	store := NewFilesystem(root)
	if err := store.Health(context.Background()); err != nil {
		t.Fatalf("Health: %v", err)
	}
	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		t.Fatalf("root not created: %v", err)
	}
}

func TestFilesystemOpenMissingKey(t *testing.T) {
	store := NewFilesystem(t.TempDir())
	_, _, err := store.Open(context.Background(), "workspaces/ws/attachments/missing")
	if err == nil {
		t.Fatal("expected missing-key error")
	}
}

func TestFilesystemContextCanceled(t *testing.T) {
	store := NewFilesystem(t.TempDir())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := store.Put(ctx, "workspaces/ws/attachments/id", strings.NewReader("x"), 1, "text/plain"); err == nil {
		t.Fatal("expected ctx error")
	}
}

package sqlite

import (
	"path/filepath"
	"testing"
)

func TestOpenInitializesLocalWorkspace(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "taskg.db")

	store, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	ws, err := store.LocalWorkspace()
	if err != nil {
		t.Fatalf("LocalWorkspace() error = %v", err)
	}
	if ws.ID == "" {
		t.Fatal("workspace ID is empty")
	}
	if ws.Slug != "local" {
		t.Fatalf("Slug = %q, want local", ws.Slug)
	}
	if ws.Name != "Local" {
		t.Fatalf("Name = %q, want Local", ws.Name)
	}
}

func TestOpenCanReopenExistingDatabase(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "taskg.db")

	store1, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open first error = %v", err)
	}
	ws1, err := store1.LocalWorkspace()
	if err != nil {
		t.Fatalf("LocalWorkspace first error = %v", err)
	}
	_ = store1.Close()

	store2, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open second error = %v", err)
	}
	t.Cleanup(func() { _ = store2.Close() })
	ws2, err := store2.LocalWorkspace()
	if err != nil {
		t.Fatalf("LocalWorkspace second error = %v", err)
	}
	if ws2.ID != ws1.ID {
		t.Fatalf("workspace ID changed: %q -> %q", ws1.ID, ws2.ID)
	}
}

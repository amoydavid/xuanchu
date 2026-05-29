package sqlite

import (
	"path/filepath"
	"testing"
)

func TestOpenInitializesLocalUserWorkspaceAndMembership(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "taskg.db")

	store, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	user, err := NewUserRepository(store.DB()).GetByName("local")
	if err != nil {
		t.Fatalf("GetByName(local) error = %v", err)
	}
	if user.Email != nil {
		t.Fatalf("local email = %q, want nil", *user.Email)
	}

	ws, err := store.LocalWorkspace()
	if err != nil {
		t.Fatalf("LocalWorkspace() error = %v", err)
	}
	if user.DefaultWorkspaceID == nil || *user.DefaultWorkspaceID != ws.ID {
		t.Fatalf("default workspace = %#v, want %s", user.DefaultWorkspaceID, ws.ID)
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

	member, err := NewMemberRepository(store.DB()).Get(user.ID, ws.ID)
	if err != nil {
		t.Fatalf("Get(local membership) error = %v", err)
	}
	if member.Role != "owner" {
		t.Fatalf("role = %q, want owner", member.Role)
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

func TestOpenMigratesContextActiveMeta(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "taskg.db")

	store, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if err := store.SetMeta("context.active", "work"); err != nil {
		t.Fatalf("SetMeta(context.active) error = %v", err)
	}
	_ = store.Close()

	reopened, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open(reopen) error = %v", err)
	}
	t.Cleanup(func() { _ = reopened.Close() })

	user, err := NewUserRepository(reopened.DB()).GetByName("local")
	if err != nil {
		t.Fatalf("GetByName(local) error = %v", err)
	}
	ws, err := reopened.LocalWorkspace()
	if err != nil {
		t.Fatalf("LocalWorkspace() error = %v", err)
	}
	if _, ok, err := reopened.GetMeta("context.active"); err != nil {
		t.Fatalf("GetMeta(context.active) error = %v", err)
	} else if ok {
		t.Fatal("old context.active key still exists")
	}
	got, ok, err := reopened.GetMeta("active_context." + user.ID + "." + ws.ID)
	if err != nil || !ok || got != "work" {
		t.Fatalf("migrated active context = %q, %v, %v", got, ok, err)
	}
}

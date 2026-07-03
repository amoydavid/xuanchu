package storage

import (
	"path/filepath"
	"testing"
)

func TestSessionCreateGetDelete(t *testing.T) {
	store, _ := Open(filepath.Join(t.TempDir(), "x.db"))
	t.Cleanup(func() { _ = store.Close() })
	repo := NewSessionRepository(store.db)

	if err := repo.CreateSession("hash1", "u1", "ws1", "csrfhash1", 100, 200); err != nil {
		t.Fatalf("create: %v", err)
	}
	s, err := repo.GetSession("hash1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if s.UserID != "u1" || s.WorkspaceID != "ws1" || s.CSRFHash != "csrfhash1" {
		t.Fatalf("got %+v", s)
	}
	if err := repo.DeleteSession("hash1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := repo.GetSession("hash1"); err != ErrNotFound {
		t.Fatalf("after delete err = %v, want ErrNotFound", err)
	}
}

func TestSessionPurgeExpired(t *testing.T) {
	store, _ := Open(filepath.Join(t.TempDir(), "x.db"))
	t.Cleanup(func() { _ = store.Close() })
	repo := NewSessionRepository(store.db)

	_ = repo.CreateSession("h1", "u1", "ws1", "c1", 1, 50)  // 已过期
	_ = repo.CreateSession("h2", "u2", "ws1", "c2", 1, 200) // 未过期
	n, err := repo.PurgeExpiredSessions(100)
	if err != nil {
		t.Fatalf("purge: %v", err)
	}
	if n != 1 {
		t.Fatalf("purged %d, want 1", n)
	}
	if _, err := repo.GetSession("h1"); err != ErrNotFound {
		t.Fatalf("h1 should be gone")
	}
}

func TestAuthFlowCreateGetDelete(t *testing.T) {
	store, _ := Open(filepath.Join(t.TempDir(), "x.db"))
	t.Cleanup(func() { _ = store.Close() })
	repo := NewSessionRepository(store.db)

	if err := repo.CreateAuthFlow("state1", "ws1", "verifier1", 100, 700); err != nil {
		t.Fatalf("create flow: %v", err)
	}
	f, err := repo.GetAuthFlow("state1")
	if err != nil {
		t.Fatalf("get flow: %v", err)
	}
	if f.PKCEVerifier != "verifier1" || f.WorkspaceID != "ws1" {
		t.Fatalf("got %+v", f)
	}
	if err := repo.DeleteAuthFlow("state1"); err != nil {
		t.Fatalf("delete flow: %v", err)
	}
	if _, err := repo.GetAuthFlow("state1"); err != ErrNotFound {
		t.Fatalf("after delete")
	}
}

func TestAuthFlowPurgeExpired(t *testing.T) {
	store, _ := Open(filepath.Join(t.TempDir(), "x.db"))
	t.Cleanup(func() { _ = store.Close() })
	repo := NewSessionRepository(store.db)

	_ = repo.CreateAuthFlow("s1", "ws1", "v", 1, 50)
	_ = repo.CreateAuthFlow("s2", "ws1", "v", 1, 200)
	n, _ := repo.PurgeExpiredAuthFlows(100)
	if n != 1 {
		t.Fatalf("purged %d, want 1", n)
	}
}

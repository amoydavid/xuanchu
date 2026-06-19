package storage

import (
	"path/filepath"
	"testing"
)

func openAdminActingSessionTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := Open(filepath.Join(t.TempDir(), "xuanchu.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func TestAdminActingSessionCreateAndGetByPrefix(t *testing.T) {
	store := openAdminActingSessionTestStore(t)
	repo := NewAdminActingSessionRepository(store.DB())
	adminTokenID := "admin-token-1"
	entry := AdminActingSessionEntry{
		ID:             "act-1",
		TokenPrefix:    "xuanchu_act_ab",
		TokenHash:      "sha256:abc",
		AdminTokenID:   &adminTokenID,
		AdminTokenName: "ops-primary",
		WorkspaceID:    "ws-1",
		ActorUserID:    "alice",
		Role:           "owner",
		CreatedAt:      10,
		ExpiresAt:      100,
	}
	if err := repo.Create(entry); err != nil {
		t.Fatal(err)
	}

	got, err := repo.GetByPrefix("xuanchu_act_ab")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "act-1" || got.WorkspaceID != "ws-1" || got.ActorUserID != "alice" {
		t.Fatalf("got = %#v", got)
	}
	if got.AdminTokenID == nil || *got.AdminTokenID != "admin-token-1" {
		t.Fatalf("admin token id = %#v", got.AdminTokenID)
	}
	if got.AdminTokenName != "ops-primary" {
		t.Fatalf("admin token name = %q", got.AdminTokenName)
	}
	if got.TokenHash == "xuanchu_act_secret" {
		t.Fatalf("stored raw token in hash field: %#v", got)
	}
}

func TestAdminActingSessionGetByIDMissingReturnsErrNotFound(t *testing.T) {
	store := openAdminActingSessionTestStore(t)
	repo := NewAdminActingSessionRepository(store.DB())
	if _, err := repo.GetByID("nope"); err != ErrNotFound {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
	if _, err := repo.GetByPrefix("xuanchu_act_zz"); err != ErrNotFound {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestAdminActingSessionListValidExcludesRevokedAndExpired(t *testing.T) {
	store := openAdminActingSessionTestStore(t)
	repo := NewAdminActingSessionRepository(store.DB())
	revokedAt := int64(50)
	rows := []AdminActingSessionEntry{
		{ID: "valid", TokenPrefix: "xuanchu_act_v1", TokenHash: "sha256:1", AdminTokenName: "ops", WorkspaceID: "ws", ActorUserID: "u1", Role: "owner", CreatedAt: 10, ExpiresAt: 100},
		{ID: "revoked", TokenPrefix: "xuanchu_act_v2", TokenHash: "sha256:2", AdminTokenName: "ops", WorkspaceID: "ws", ActorUserID: "u1", Role: "owner", CreatedAt: 10, ExpiresAt: 100, RevokedAt: &revokedAt},
		{ID: "expired", TokenPrefix: "xuanchu_act_v3", TokenHash: "sha256:3", AdminTokenName: "ops", WorkspaceID: "ws", ActorUserID: "u1", Role: "owner", CreatedAt: 10, ExpiresAt: 30},
	}
	for _, row := range rows {
		if err := repo.Create(row); err != nil {
			t.Fatal(err)
		}
	}

	valid, err := repo.ListValid(60)
	if err != nil {
		t.Fatal(err)
	}
	if len(valid) != 1 || valid[0].ID != "valid" {
		t.Fatalf("valid rows = %#v", valid)
	}
}

func TestAdminActingSessionTouchLastUsedAndRevoke(t *testing.T) {
	store := openAdminActingSessionTestStore(t)
	repo := NewAdminActingSessionRepository(store.DB())
	if err := repo.Create(AdminActingSessionEntry{
		ID:             "act-1",
		TokenPrefix:    "xuanchu_act_ab",
		TokenHash:      "sha256:abc",
		AdminTokenName: "ops",
		WorkspaceID:    "ws",
		ActorUserID:    "alice",
		Role:           "owner",
		CreatedAt:      10,
		ExpiresAt:      100,
	}); err != nil {
		t.Fatal(err)
	}

	if err := repo.TouchLastUsed("act-1", 77); err != nil {
		t.Fatal(err)
	}
	got, err := repo.GetByID("act-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.LastUsedAt == nil || *got.LastUsedAt != 77 {
		t.Fatalf("last_used_at = %#v", got.LastUsedAt)
	}

	if err := repo.Revoke("act-1", 88); err != nil {
		t.Fatal(err)
	}
	got, err = repo.GetByID("act-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.RevokedAt == nil || *got.RevokedAt != 88 {
		t.Fatalf("revoked_at = %#v", got.RevokedAt)
	}

	// 对不存在行的更新应返回 ErrNotFound。
	if err := repo.TouchLastUsed("missing", 1); err != ErrNotFound {
		t.Fatalf("touch missing err = %v, want ErrNotFound", err)
	}
	if err := repo.Revoke("missing", 1); err != ErrNotFound {
		t.Fatalf("revoke missing err = %v, want ErrNotFound", err)
	}
}

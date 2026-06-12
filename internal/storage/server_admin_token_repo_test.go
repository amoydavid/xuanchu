package storage

import (
	"path/filepath"
	"testing"
)

func openServerAdminTokenTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := Open(filepath.Join(t.TempDir(), "xuanchu.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func TestServerAdminTokenRepositoryCreateAndListValid(t *testing.T) {
	store := openServerAdminTokenTestStore(t)
	repo := NewServerAdminTokenRepository(store.DB())
	revokedAt := int64(40)
	rows := []ServerAdminTokenEntry{
		{ID: "enabled", Name: "primary", TokenPrefix: "xuanchu_admin_a", TokenHash: "sha256:a", Enabled: true, CreatedAt: 10},
		{ID: "disabled", Name: "disabled", TokenPrefix: "xuanchu_admin_b", TokenHash: "sha256:b", Enabled: false, CreatedAt: 20},
		{ID: "revoked", Name: "revoked", TokenPrefix: "xuanchu_admin_c", TokenHash: "sha256:c", Enabled: true, CreatedAt: 30, RevokedAt: &revokedAt},
	}
	for _, row := range rows {
		if err := repo.Create(row); err != nil {
			t.Fatal(err)
		}
	}

	valid, err := repo.ListValid()
	if err != nil {
		t.Fatal(err)
	}
	if len(valid) != 1 || valid[0].ID != "enabled" {
		t.Fatalf("valid rows = %#v", valid)
	}
	if valid[0].TokenHash == "xuanchu_admin_secret" {
		t.Fatalf("stored raw token in hash field: %#v", valid[0])
	}
}

func TestServerAdminTokenRepositoryGetByPrefixAndTouch(t *testing.T) {
	store := openServerAdminTokenTestStore(t)
	repo := NewServerAdminTokenRepository(store.DB())
	entry := ServerAdminTokenEntry{
		ID:          "admin-1",
		Name:        "primary",
		TokenPrefix: "xuanchu_admin_1",
		TokenHash:   "sha256:abc",
		Enabled:     true,
		CreatedAt:   10,
		Description: "initial token",
	}
	if err := repo.Create(entry); err != nil {
		t.Fatal(err)
	}

	got, err := repo.GetByPrefix("xuanchu_admin_1")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != entry.ID || got.Description != entry.Description {
		t.Fatalf("got = %#v", got)
	}

	if err := repo.TouchLastUsed(entry.ID, 99); err != nil {
		t.Fatal(err)
	}
	got, err = repo.GetByPrefix("xuanchu_admin_1")
	if err != nil {
		t.Fatal(err)
	}
	if got.LastUsedAt == nil || *got.LastUsedAt != 99 {
		t.Fatalf("last_used_at = %#v", got.LastUsedAt)
	}
}

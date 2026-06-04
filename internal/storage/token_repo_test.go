package storage

import (
	"path/filepath"
	"strings"
	"testing"
)

func newTokenRepoTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := Open(filepath.Join(t.TempDir(), "taskg.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func TestOpenCreatesAPITokenSchema(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "taskg.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if !store.DB().Migrator().HasTable(&ApiToken{}) {
		t.Fatalf("missing api_tokens table")
	}
	assertIndexColumns(t, store, "idx_api_tokens_user", []string{"user_id"})
	assertIndexColumns(t, store, "idx_api_tokens_prefix", []string{"token_prefix"})
}

func TestTokenRepositoryDoesNotStoreRawToken(t *testing.T) {
	store := newTokenRepoTestStore(t)
	repo := NewTokenRepository(store.DB())
	row := ApiTokenEntry{
		ID:               "tok1",
		UserID:           "u1",
		Name:             "cli",
		Type:             "pat",
		TokenPrefix:      "taskg_pat_abcd",
		TokenHash:        strings.Repeat("a", 64),
		ScopesJSON:       `["task:read"]`,
		WorkspaceIDsJSON: `[]`,
		ProjectIDsJSON:   `[]`,
		CreatedAt:        100,
	}
	if err := repo.Create(row); err != nil {
		t.Fatal(err)
	}
	got, err := repo.GetByPrefix("taskg_pat_abcd")
	if err != nil {
		t.Fatal(err)
	}
	if got.TokenHash != row.TokenHash {
		t.Fatalf("hash mismatch")
	}
	if strings.Contains(got.TokenHash, "taskg_pat_") {
		t.Fatalf("raw token leaked")
	}
}

func TestTokenRepositoryRevokeAndListByUser(t *testing.T) {
	store := newTokenRepoTestStore(t)
	repo := NewTokenRepository(store.DB())
	for _, row := range []ApiTokenEntry{
		{
			ID:               "tok1",
			UserID:           "u1",
			Name:             "cli",
			Type:             "pat",
			TokenPrefix:      "taskg_pat_a1",
			TokenHash:        strings.Repeat("a", 64),
			ScopesJSON:       `["task:read"]`,
			WorkspaceIDsJSON: `[]`,
			ProjectIDsJSON:   `[]`,
			CreatedAt:        100,
		},
		{
			ID:               "tok2",
			UserID:           "u1",
			Name:             "agent",
			Type:             "agent",
			TokenPrefix:      "taskg_agent_b2",
			TokenHash:        strings.Repeat("b", 64),
			ScopesJSON:       `["task:read"]`,
			WorkspaceIDsJSON: `["w1"]`,
			ProjectIDsJSON:   `[]`,
			CreatedAt:        101,
		},
	} {
		if err := repo.Create(row); err != nil {
			t.Fatalf("Create(%s) error = %v", row.ID, err)
		}
	}
	if err := repo.Revoke("tok1", 200); err != nil {
		t.Fatalf("Revoke() error = %v", err)
	}
	active, err := repo.ListByUser("u1", false)
	if err != nil {
		t.Fatalf("ListByUser(active) error = %v", err)
	}
	if len(active) != 1 || active[0].ID != "tok2" {
		t.Fatalf("active tokens = %#v", active)
	}
	all, err := repo.ListByUser("u1", true)
	if err != nil {
		t.Fatalf("ListByUser(all) error = %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("all tokens = %#v", all)
	}
	if all[1].RevokedAt == nil || *all[1].RevokedAt != 200 {
		t.Fatalf("revoked token = %#v", all[1])
	}
}

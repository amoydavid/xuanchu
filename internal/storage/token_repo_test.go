package storage

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func newTokenRepoTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := Open(filepath.Join(t.TempDir(), "xuanchu.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func TestOpenCreatesAPITokenSchema(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "xuanchu.db"))
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
		TokenPrefix:      "xuanchu_pat_abcd",
		TokenHash:        strings.Repeat("a", 64),
		ScopesJSON:       `["task:read"]`,
		WorkspaceIDsJSON: `[]`,
		ProjectIDsJSON:   `[]`,
		CreatedAt:        100,
	}
	if err := repo.Create(row); err != nil {
		t.Fatal(err)
	}
	got, err := repo.GetByPrefix("xuanchu_pat_abcd")
	if err != nil {
		t.Fatal(err)
	}
	if got.TokenHash != row.TokenHash {
		t.Fatalf("hash mismatch")
	}
	if strings.Contains(got.TokenHash, "xuanchu_pat_") {
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
			TokenPrefix:      "xuanchu_pat_a1",
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
			TokenPrefix:      "xuanchu_agent_b2",
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

func TestTokenRepository_Update(t *testing.T) {
	store := newTokenRepoTestStore(t)
	repo := NewTokenRepository(store.DB())
	entry := ApiTokenEntry{
		ID:               uuid.NewString(),
		UserID:           "user1",
		Name:             "test-token",
		Type:             "pat",
		TokenPrefix:      "xuanchu_pat_abc",
		TokenHash:        "hash",
		ScopesJSON:       `["task:read"]`,
		WorkspaceIDsJSON: `[]`,
		ProjectIDsJSON:   `[]`,
		CreatedAt:        time.Now().Unix(),
	}
	if err := repo.Create(entry); err != nil {
		t.Fatal(err)
	}
	newName := "renamed"
	newScopes := `["task:read","task:write"]`
	err := repo.Update(entry.ID, TokenUpdates{
		Name:       &newName,
		ScopesJSON: &newScopes,
	})
	if err != nil {
		t.Fatal(err)
	}
	updated, err := repo.GetByID(entry.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Name != "renamed" {
		t.Fatalf("name = %q", updated.Name)
	}
	if updated.ScopesJSON != newScopes {
		t.Fatalf("scopes = %q", updated.ScopesJSON)
	}
}

func TestTokenRepository_Update_ClearExpiresAt(t *testing.T) {
	store := newTokenRepoTestStore(t)
	repo := NewTokenRepository(store.DB())
	expiresAt := time.Now().Add(24 * time.Hour).Unix()
	entry := ApiTokenEntry{
		ID:               uuid.NewString(),
		UserID:           "user1",
		Name:             "test-token",
		Type:             "pat",
		TokenPrefix:      "xuanchu_pat_abc",
		TokenHash:        "hash",
		ScopesJSON:       `["task:read"]`,
		WorkspaceIDsJSON: `[]`,
		ProjectIDsJSON:   `[]`,
		CreatedAt:        time.Now().Unix(),
		ExpiresAt:        &expiresAt,
	}
	if err := repo.Create(entry); err != nil {
		t.Fatal(err)
	}
	if err := repo.Update(entry.ID, TokenUpdates{ClearExpiresAt: true}); err != nil {
		t.Fatal(err)
	}
	updated, err := repo.GetByID(entry.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.ExpiresAt != nil {
		t.Fatalf("expected nil ExpiresAt, got %d", *updated.ExpiresAt)
	}
}

func TestTokenUpdatesChangedFieldsIncludesWorkspaceAndProject(t *testing.T) {
	name := "renamed"
	scopes := `["task:read"]`
	workspaces := `["w1","w2"]`
	projects := `["p1"]`
	expires := int64(999)

	// 全字段
	got := TokenUpdates{
		Name:             &name,
		ScopesJSON:       &scopes,
		WorkspaceIDsJSON: &workspaces,
		ProjectIDsJSON:   &projects,
		ExpiresAt:        &expires,
	}.ChangedFields()
	if got["name"] != name {
		t.Fatalf("name = %v", got["name"])
	}
	if got["scopes"] != scopes {
		t.Fatalf("scopes = %v", got["scopes"])
	}
	if got["workspace_ids"] != workspaces {
		t.Fatalf("workspace_ids = %v", got["workspace_ids"])
	}
	if got["project_ids"] != projects {
		t.Fatalf("project_ids = %v", got["project_ids"])
	}
	if got["expires_at"] != expires {
		t.Fatalf("expires_at = %v", got["expires_at"])
	}

	// 仅 workspace/project，不含其它字段
	got = TokenUpdates{
		WorkspaceIDsJSON: &workspaces,
		ProjectIDsJSON:   &projects,
	}.ChangedFields()
	if _, ok := got["name"]; ok {
		t.Fatalf("unexpected name field")
	}
	if _, ok := got["scopes"]; ok {
		t.Fatalf("unexpected scopes field")
	}
	if _, ok := got["expires_at"]; ok {
		t.Fatalf("unexpected expires_at field")
	}
	if got["workspace_ids"] != workspaces {
		t.Fatalf("workspace_ids = %v", got["workspace_ids"])
	}
	if got["project_ids"] != projects {
		t.Fatalf("project_ids = %v", got["project_ids"])
	}

	// ClearExpiresAt 输出 nil
	got = TokenUpdates{ClearExpiresAt: true}.ChangedFields()
	if got["expires_at"] != nil {
		t.Fatalf("expected nil expires_at, got %v", got["expires_at"])
	}
}

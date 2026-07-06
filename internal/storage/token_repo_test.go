package storage

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"git.dajee.net/dajee/xuanchu/internal/auth"
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
		UserID:           ptrString("u1"),
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
			UserID:           ptrString("u1"),
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
			UserID:           ptrString("u1"),
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

func TestTokenRepositoryListAll(t *testing.T) {
	store := newTokenRepoTestStore(t)
	repo := NewTokenRepository(store.DB())
	revokedAt := int64(999)
	// 建 3 个 token：2 个有效（不同 user）、1 个已吊销
	rows := []ApiTokenEntry{
		{ID: "tok1", UserID: ptrString("u1"), Name: "a", Type: "pat", TokenPrefix: "xuanchu_pat_a1", TokenHash: strings.Repeat("a", 64), ScopesJSON: `["task:read"]`, WorkspaceIDsJSON: `[]`, ProjectIDsJSON: `[]`, CreatedAt: 100},
		{ID: "tok2", UserID: ptrString("u2"), Name: "b", Type: "agent", TokenPrefix: "xuanchu_agent_b2", TokenHash: strings.Repeat("b", 64), ScopesJSON: `["task:read"]`, WorkspaceIDsJSON: `["w1"]`, ProjectIDsJSON: `[]`, CreatedAt: 200},
		{ID: "tok3", UserID: ptrString("u1"), Name: "c", Type: "pat", TokenPrefix: "xuanchu_pat_c3", TokenHash: strings.Repeat("c", 64), ScopesJSON: `["task:read"]`, WorkspaceIDsJSON: `[]`, ProjectIDsJSON: `[]`, CreatedAt: 50, RevokedAt: &revokedAt},
	}
	for _, row := range rows {
		if err := repo.Create(row); err != nil {
			t.Fatalf("Create(%s) error = %v", row.ID, err)
		}
	}

	// includeRevoked=true：返回全部 3 个，按 created_at DESC 排序（tok2=200, tok1=100, tok3=50）
	all, err := repo.ListAll(true)
	if err != nil {
		t.Fatalf("ListAll(true) error = %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("ListAll(true) len = %d, want 3: %#v", len(all), all)
	}
	if all[0].ID != "tok2" || all[2].ID != "tok3" {
		t.Fatalf("ListAll(true) order = %s,%s,%s, want tok2,tok1,tok3", all[0].ID, all[1].ID, all[2].ID)
	}

	// includeRevoked=false：过滤掉 tok3
	active, err := repo.ListAll(false)
	if err != nil {
		t.Fatalf("ListAll(false) error = %v", err)
	}
	if len(active) != 2 {
		t.Fatalf("ListAll(false) len = %d, want 2", len(active))
	}
	for _, row := range active {
		if row.ID == "tok3" {
			t.Fatalf("ListAll(false) should exclude revoked tok3")
		}
	}
}

func TestTokenRepositoryTenantTokenUsesNullableUserID(t *testing.T) {
	store := newTokenRepoTestStore(t)
	repo := NewTokenRepository(store.DB())
	wsID := "ws-1"
	userID := "user-1"
	userToken := ApiTokenEntry{
		ID:               "pat-1",
		UserID:           ptrString(userID),
		Name:             "cli",
		Type:             auth.TokenTypePAT,
		TokenPrefix:      "xuanchu_pat_a",
		TokenHash:        strings.Repeat("a", 64),
		ScopesJSON:       `["task:read"]`,
		WorkspaceIDsJSON: `[]`,
		ProjectIDsJSON:   `[]`,
		CreatedAt:        100,
	}
	tenantToken := ApiTokenEntry{
		ID:               "tenant-1",
		UserID:           nil,
		Name:             "runtime",
		Type:             auth.TokenTypeTenantAccess,
		TokenPrefix:      "xuanchu_tenant_a",
		TokenHash:        strings.Repeat("b", 64),
		ScopesJSON:       `["task:read"]`,
		WorkspaceIDsJSON: `["` + wsID + `"]`,
		ProjectIDsJSON:   `[]`,
		CreatedAt:        101,
	}
	if err := repo.Create(userToken); err != nil {
		t.Fatal(err)
	}
	if err := repo.Create(tenantToken); err != nil {
		t.Fatal(err)
	}
	byUser, err := repo.ListByUser(userID, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(byUser) != 1 || byUser[0].ID != "pat-1" {
		t.Fatalf("ListByUser() = %#v", byUser)
	}
	tenants, err := repo.ListTenantByWorkspace(wsID, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(tenants) != 1 || tenants[0].UserID != nil {
		t.Fatalf("ListTenantByWorkspace() = %#v", tenants)
	}
}

func TestTokenRepository_Update(t *testing.T) {
	store := newTokenRepoTestStore(t)
	repo := NewTokenRepository(store.DB())
	entry := ApiTokenEntry{
		ID:               uuid.NewString(),
		UserID:           ptrString("user1"),
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
		UserID:           ptrString("user1"),
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

func TestTokenRepositoryPersistsTokenSecretCiphertext(t *testing.T) {
	store := newTokenRepoTestStore(t)
	repo := NewTokenRepository(store.DB())
	entry := ApiTokenEntry{
		ID:                    "tok-secret",
		UserID:                ptrString("user-1"),
		Name:                  "agent",
		Type:                  "agent",
		TokenPrefix:           "xuanchu_agent_secret",
		TokenHash:             strings.Repeat("a", 64),
		TokenSecretCiphertext: "enc:v1:ciphertext",
		ScopesJSON:            `["task:read"]`,
		WorkspaceIDsJSON:      `["ws-1"]`,
		ProjectIDsJSON:        `[]`,
		CreatedAt:             100,
	}
	if err := repo.Create(entry); err != nil {
		t.Fatal(err)
	}
	got, err := repo.GetByID(entry.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.TokenSecretCiphertext != entry.TokenSecretCiphertext {
		t.Fatalf("ciphertext = %q, want %q", got.TokenSecretCiphertext, entry.TokenSecretCiphertext)
	}
}

func TestTokenRepositoryPersistsWebLoginDisabled(t *testing.T) {
	store := newTokenRepoTestStore(t)
	repo := NewTokenRepository(store.DB())
	entry := ApiTokenEntry{
		ID:               "tok-weblogin",
		UserID:           ptrString("user-1"),
		Name:             "sso-pat",
		Type:             "pat",
		TokenPrefix:      "xuanchu_pat_weblogin",
		TokenHash:        strings.Repeat("a", 64),
		ScopesJSON:       `["task:read"]`,
		WorkspaceIDsJSON: `[]`,
		ProjectIDsJSON:   `[]`,
		WebLoginDisabled: true,
		CreatedAt:        100,
	}
	if err := repo.Create(entry); err != nil {
		t.Fatal(err)
	}
	got, err := repo.GetByID(entry.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !got.WebLoginDisabled {
		t.Fatalf("WebLoginDisabled = false, want true")
	}

	// 未设置时默认为 false
	plain := ApiTokenEntry{
		ID:               "tok-plain",
		UserID:           ptrString("user-1"),
		Name:             "cli-pat",
		Type:             "pat",
		TokenPrefix:      "xuanchu_pat_plain",
		TokenHash:        strings.Repeat("b", 64),
		ScopesJSON:       `["task:read"]`,
		WorkspaceIDsJSON: `[]`,
		ProjectIDsJSON:   `[]`,
		CreatedAt:        100,
	}
	if err := repo.Create(plain); err != nil {
		t.Fatal(err)
	}
	gotPlain, err := repo.GetByID(plain.ID)
	if err != nil {
		t.Fatal(err)
	}
	if gotPlain.WebLoginDisabled {
		t.Fatalf("WebLoginDisabled = true, want false by default")
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

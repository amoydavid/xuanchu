package app

import (
	"encoding/json"
	"testing"

	"git.dajee.net/dajee/xuanchu/internal/storage"
)

func TestAdminCreateWorkspaceCreatesOwnerMembership(t *testing.T) {
	store := newTestStore(t)
	svc := newAdminTestService(t, store)
	result, err := svc.AdminCreateWorkspace(AdminCreateWorkspaceInput{
		AdminTokenName: "ops",
		Slug:           "dajee",
		Name:           "Dajee",
		Visibility:     "team",
		Owner:          AdminOwnerInput{Name: "alice", Email: "alice@example.com"},
	})
	if err != nil {
		t.Fatalf("AdminCreateWorkspace() error = %v", err)
	}
	if result.Workspace.Slug != "dajee" || result.Owner.Name != "alice" {
		t.Fatalf("result = %#v", result)
	}
	member, err := storage.NewMemberRepository(store.DB()).Get(result.Owner.ID, result.Workspace.ID)
	if err != nil {
		t.Fatalf("membership missing: %v", err)
	}
	if member.Role != string(RoleOwner) {
		t.Fatalf("role = %q, want owner", member.Role)
	}
}

func TestAdminCreateWorkspaceWritesAuditWithoutRawToken(t *testing.T) {
	store := newTestStore(t)
	svc := newAdminTestService(t, store)
	_, err := svc.AdminCreateWorkspace(AdminCreateWorkspaceInput{
		AdminTokenName: "ops",
		Slug:           "auditws",
		Owner:          AdminOwnerInput{Name: "alice"},
	})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := storage.NewAuditRepository(store.DB()).List(storage.AuditListOptions{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) == 0 || rows[0].Action != "admin.workspace.create" {
		t.Fatalf("audit rows = %#v", rows)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(rows[0].PayloadJSON), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["admin_token_name"] != "ops" || payload["admin"] != true {
		t.Fatalf("payload = %#v", payload)
	}
	if _, ok := payload["token"]; ok {
		t.Fatalf("payload leaked token: %#v", payload)
	}
}

func TestAdminCreateWorkspaceAgentToken(t *testing.T) {
	store := newTestStore(t)
	svc := newAdminTestService(t, store)
	ws, err := svc.AdminCreateWorkspace(AdminCreateWorkspaceInput{
		AdminTokenName: "ops",
		Slug:           "dajee",
		Owner:          AdminOwnerInput{Name: "alice", Email: "alice@example.com"},
	})
	if err != nil {
		t.Fatal(err)
	}
	created, err := svc.AdminCreateWorkspaceAgentToken(AdminCreateAgentTokenInput{
		AdminTokenName: "ops",
		WorkspaceRef:   "dajee",
		Name:           "openclaw",
		UserRef:        "alice@example.com",
		Scopes:         []string{"task:read", "task:write"},
	})
	if err != nil {
		t.Fatalf("AdminCreateWorkspaceAgentToken() error = %v", err)
	}
	if created.RawToken == "" || created.View.Type != "agent" {
		t.Fatalf("created = %#v", created)
	}
	if len(created.View.WorkspaceIDs) != 1 || created.View.WorkspaceIDs[0] != ws.Workspace.ID {
		t.Fatalf("workspace ids = %#v, want %s", created.View.WorkspaceIDs, ws.Workspace.ID)
	}
}

func TestAdminCreateWorkspaceAgentTokenRejectsNonMember(t *testing.T) {
	store := newTestStore(t)
	svc := newAdminTestService(t, store)
	_, err := svc.AdminCreateWorkspace(AdminCreateWorkspaceInput{
		AdminTokenName: "ops",
		Slug:           "dajee",
		Owner:          AdminOwnerInput{Name: "alice"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddUser(AddUserInput{Name: "bob"}); err != nil {
		t.Fatal(err)
	}
	_, err = svc.AdminCreateWorkspaceAgentToken(AdminCreateAgentTokenInput{
		AdminTokenName: "ops",
		WorkspaceRef:   "dajee",
		Name:           "bad",
		UserRef:        "bob",
		Scopes:         []string{"task:read"},
	})
	if err == nil {
		t.Fatal("AdminCreateWorkspaceAgentToken(non-member) error = nil")
	}
}

func TestAdminCreateWorkspaceAgentTokenRejectsInvalidScope(t *testing.T) {
	store := newTestStore(t)
	svc := newAdminTestService(t, store)
	_, err := svc.AdminCreateWorkspace(AdminCreateWorkspaceInput{
		AdminTokenName: "ops",
		Slug:           "dajee",
		Owner:          AdminOwnerInput{Name: "alice"},
	})
	if err != nil {
		t.Fatal(err)
	}
	// bogus:read 不在 scopeRegistry，应被 createTokenStored 的 ValidateTokenCreate 拒绝。
	_, err = svc.AdminCreateWorkspaceAgentToken(AdminCreateAgentTokenInput{
		AdminTokenName: "ops",
		WorkspaceRef:   "dajee",
		Name:           "bad-scope",
		UserRef:        "alice",
		Scopes:         []string{"task:read", "bogus:read"},
	})
	assertRuntimeCode(t, err, "token_scope_invalid")
}

func TestAdminCreateWorkspaceRejectsOwnerNameEmailConflict(t *testing.T) {
	store := newTestStore(t)
	svc := newAdminTestService(t, store)
	email := "alice@example.com"
	mustCreateUserRecord(t, store, storage.User{ID: "user-alice-admin", Name: "alice", CreatedAt: 100, ModifiedAt: 100})
	mustCreateUserRecord(t, store, storage.User{ID: "user-other-admin", Name: "other", Email: &email, CreatedAt: 100, ModifiedAt: 100})
	_, err := svc.AdminCreateWorkspace(AdminCreateWorkspaceInput{
		AdminTokenName: "ops",
		Slug:           "conflict",
		Owner:          AdminOwnerInput{Name: "alice", Email: email},
	})
	assertRuntimeCode(t, err, "admin_owner_invalid")
}

func TestAdminCreateWorkspaceAdminCreatesOwnerMembershipAndAudit(t *testing.T) {
	store := newTestStore(t)
	svc := newAdminTestService(t, store)
	ws, err := svc.AdminCreateWorkspace(AdminCreateWorkspaceInput{
		AdminTokenName: "ops",
		Slug:           "dajee",
		Owner:          AdminOwnerInput{Name: "root"},
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := svc.AdminCreateWorkspaceAdmin(AdminCreateWorkspaceAdminInput{
		AdminTokenName: "ops",
		WorkspaceRef:   "dajee",
		Name:           "alice",
		Email:          "alice@example.com",
		Role:           RoleOwner,
	})
	if err != nil {
		t.Fatalf("AdminCreateWorkspaceAdmin() error = %v", err)
	}
	if result.Workspace.Slug != "dajee" || result.Admin.Name != "alice" {
		t.Fatalf("result = %#v", result)
	}
	member, err := storage.NewMemberRepository(store.DB()).Get(result.Admin.ID, ws.Workspace.ID)
	if err != nil {
		t.Fatalf("membership missing: %v", err)
	}
	if member.Role != string(RoleOwner) {
		t.Fatalf("role = %q, want owner", member.Role)
	}
	rows, err := storage.NewAuditRepository(store.DB()).List(storage.AuditListOptions{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	var found *storage.AuditLogEntry
	for i := range rows {
		if rows[i].Action == "admin.workspace_admin.create" {
			found = &rows[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("audit rows = %#v", rows)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(found.PayloadJSON), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["admin_token_name"] != "ops" || payload["admin"] != true {
		t.Fatalf("payload = %#v", payload)
	}
	if _, ok := payload["token"]; ok {
		t.Fatalf("payload leaked token: %#v", payload)
	}
}

func TestAdminCreateWorkspaceAdminPromotesExistingMembers(t *testing.T) {
	store := newTestStore(t)
	svc := newAdminTestService(t, store)
	ws, err := svc.AdminCreateWorkspace(AdminCreateWorkspaceInput{
		AdminTokenName: "ops",
		Slug:           "dajee",
		Owner:          AdminOwnerInput{Name: "root"},
	})
	if err != nil {
		t.Fatal(err)
	}
	memberRepo := storage.NewMemberRepository(store.DB())
	bob := mustCreateUserRecord(t, store, storage.User{ID: "user-bob-admin", Name: "bob", CreatedAt: 100, ModifiedAt: 100})
	if err := memberRepo.Upsert(storage.Membership{UserID: bob.ID, WorkspaceID: ws.Workspace.ID, Role: string(RoleMember), JoinedAt: 100, ModifiedAt: 100}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AdminCreateWorkspaceAdmin(AdminCreateWorkspaceAdminInput{
		AdminTokenName: "ops",
		WorkspaceRef:   "dajee",
		Name:           "bob",
		Role:           RoleAdmin,
	}); err != nil {
		t.Fatalf("AdminCreateWorkspaceAdmin(member to admin) error = %v", err)
	}
	member, err := memberRepo.Get(bob.ID, ws.Workspace.ID)
	if err != nil {
		t.Fatal(err)
	}
	if member.Role != string(RoleAdmin) {
		t.Fatalf("bob role = %q, want admin", member.Role)
	}
	if _, err := svc.AdminCreateWorkspaceAdmin(AdminCreateWorkspaceAdminInput{
		AdminTokenName: "ops",
		WorkspaceRef:   "dajee",
		Name:           "bob",
		Role:           RoleOwner,
	}); err != nil {
		t.Fatalf("AdminCreateWorkspaceAdmin(admin to owner) error = %v", err)
	}
	member, err = memberRepo.Get(bob.ID, ws.Workspace.ID)
	if err != nil {
		t.Fatal(err)
	}
	if member.Role != string(RoleOwner) {
		t.Fatalf("bob role = %q, want owner", member.Role)
	}
	rows, err := storage.NewAuditRepository(store.DB()).List(storage.AuditListOptions{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	foundPromote := false
	for _, row := range rows {
		if row.Action == "admin.workspace_admin.promote" {
			foundPromote = true
			break
		}
	}
	if !foundPromote {
		t.Fatalf("audit rows = %#v", rows)
	}
}

func TestAdminCreateWorkspaceAdminRejectsInvalidInputs(t *testing.T) {
	store := newTestStore(t)
	svc := newAdminTestService(t, store)
	ws, err := svc.AdminCreateWorkspace(AdminCreateWorkspaceInput{
		AdminTokenName: "ops",
		Slug:           "dajee",
		Owner:          AdminOwnerInput{Name: "root"},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, role := range []Role{RoleMember, RoleViewer} {
		_, err := svc.AdminCreateWorkspaceAdmin(AdminCreateWorkspaceAdminInput{
			AdminTokenName: "ops",
			WorkspaceRef:   "dajee",
			Name:           "alice",
			Role:           role,
		})
		assertRuntimeCode(t, err, "admin_role_invalid")
	}
	_, err = svc.AdminCreateWorkspaceAdmin(AdminCreateWorkspaceAdminInput{
		AdminTokenName: "ops",
		WorkspaceRef:   "missing",
		Name:           "alice",
		Role:           RoleOwner,
	})
	assertRuntimeCode(t, err, "workspace_not_found")
	now := int64(1234)
	if err := store.DB().Model(&storage.Workspace{}).Where("id = ?", ws.Workspace.ID).Update("archived_at", now).Error; err != nil {
		t.Fatal(err)
	}
	_, err = svc.AdminCreateWorkspaceAdmin(AdminCreateWorkspaceAdminInput{
		AdminTokenName: "ops",
		WorkspaceRef:   "dajee",
		Name:           "alice",
		Role:           RoleOwner,
	})
	assertRuntimeCode(t, err, "workspace_archived")
}

func TestAdminCreateWorkspaceAdminRejectsNameEmailConflictAndOwnerDowngrade(t *testing.T) {
	store := newTestStore(t)
	svc := newAdminTestService(t, store)
	_, err := svc.AdminCreateWorkspace(AdminCreateWorkspaceInput{
		AdminTokenName: "ops",
		Slug:           "dajee",
		Owner:          AdminOwnerInput{Name: "root"},
	})
	if err != nil {
		t.Fatal(err)
	}
	email := "alice@example.com"
	mustCreateUserRecord(t, store, storage.User{ID: "user-alice-admin", Name: "alice", CreatedAt: 100, ModifiedAt: 100})
	mustCreateUserRecord(t, store, storage.User{ID: "user-other-admin", Name: "other", Email: &email, CreatedAt: 100, ModifiedAt: 100})
	_, err = svc.AdminCreateWorkspaceAdmin(AdminCreateWorkspaceAdminInput{
		AdminTokenName: "ops",
		WorkspaceRef:   "dajee",
		Name:           "alice",
		Email:          email,
		Role:           RoleOwner,
	})
	assertRuntimeCode(t, err, "admin_owner_invalid")
	_, err = svc.AdminCreateWorkspaceAdmin(AdminCreateWorkspaceAdminInput{
		AdminTokenName: "ops",
		WorkspaceRef:   "dajee",
		Name:           "root",
		Role:           RoleAdmin,
	})
	assertRuntimeCode(t, err, "admin_role_invalid")
}

func newAdminTestService(t *testing.T, store *storage.Store) *Service {
	t.Helper()
	svc, err := NewService(ServiceOptions{
		Store:                 store,
		Runtime:               &RuntimeContext{ActorName: "server-admin"},
		DisableScopeBootstrap: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	return svc
}

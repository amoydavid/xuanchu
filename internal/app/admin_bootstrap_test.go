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

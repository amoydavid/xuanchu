package app

import (
	"encoding/json"
	"testing"
	"time"

	"git.dajee.net/dajee/xuanchu/internal/storage"
)

func TestAdminListTokens(t *testing.T) {
	store := newTestStore(t)
	ownerSvc := newTestServiceWithRuntime(t, store, 100, "local", "local")

	// 建第二个 user + workspace，构造多 user 多 workspace 的 token
	otherEmail := "other@example.com"
	other := mustCreateUserRecord(t, store, storage.User{
		ID: "user-other", Name: "other", Email: &otherEmail, CreatedAt: 100, ModifiedAt: 100,
	})
	team, err := ownerSvc.AddWorkspace(AddWorkspaceInput{Slug: "team", Name: "Team"})
	if err != nil {
		t.Fatal(err)
	}
	mustUpsertMembershipRecord(t, store, storage.Membership{
		UserID: other.ID, WorkspaceID: team.ID,
		Role: string(RoleMember), JoinedAt: 100, ModifiedAt: 100,
	})

	// owner 的 PAT（全局，无 workspace）
	_, err = ownerSvc.CreateToken(CreateTokenInput{
		Name:   "owner-pat",
		Type:   "pat",
		Scopes: []string{"task:read"},
	})
	if err != nil {
		t.Fatal(err)
	}
	// owner 在 local 的 agent token
	_, err = ownerSvc.CreateToken(CreateTokenInput{
		Name:          "local-agent",
		Type:          "agent",
		Scopes:        []string{"task:read", "task:write"},
		WorkspaceRefs: []string{"local"},
	})
	if err != nil {
		t.Fatal(err)
	}

	// admin service（不携带 workspace role）
	adminSvc, err := NewService(ServiceOptions{Store: store, Clock: FixedClock{NowUnix: 100}})
	if err != nil {
		t.Fatal(err)
	}

	// ListAll 含已吊销前先吊销一个
	patTokens, _ := adminSvc.AdminListTokens(false)
	if len(patTokens) < 2 {
		t.Fatalf("AdminListTokens(false) len = %d, want >= 2", len(patTokens))
	}
	// 找到 local-agent 吊销
	var agentID string
	for _, tk := range patTokens {
		if tk.Name == "local-agent" {
			agentID = tk.ID
		}
	}
	if err := adminSvc.AdminRevokeToken(agentID, "admin-ops"); err != nil {
		t.Fatalf("AdminRevokeToken() error = %v", err)
	}

	// 不含已吊销
	active, err := adminSvc.AdminListTokens(false)
	if err != nil {
		t.Fatal(err)
	}
	for _, tk := range active {
		if tk.ID == agentID {
			t.Fatalf("AdminListTokens(false) should exclude revoked %s", agentID)
		}
	}

	// 含已吊销
	all, err := adminSvc.AdminListTokens(true)
	if err != nil {
		t.Fatal(err)
	}
	foundRevoked := false
	for _, tk := range all {
		if tk.ID == agentID {
			foundRevoked = true
		}
	}
	if !foundRevoked {
		t.Fatalf("AdminListTokens(true) should include revoked %s", agentID)
	}
}

func TestAdminListTokensFillsUserInfo(t *testing.T) {
	store := newTestStore(t)
	ownerSvc := newTestServiceWithRuntime(t, store, 100, "local", "local")

	_, err := ownerSvc.CreateToken(CreateTokenInput{
		Name:   "t",
		Type:   "pat",
		Scopes: []string{"task:read"},
	})
	if err != nil {
		t.Fatal(err)
	}

	adminSvc, err := NewService(ServiceOptions{Store: store, Clock: FixedClock{NowUnix: 100}})
	if err != nil {
		t.Fatal(err)
	}

	rows, err := adminSvc.AdminListTokens(false)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) == 0 {
		t.Fatal("expected tokens")
	}
	// user info 应含 name（owner 默认是 "local"）
	if rows[0].User.Name == "" || rows[0].User.ID == "" {
		t.Fatalf("user info not filled: %+v", rows[0].User)
	}
	if rows[0].User.Name != "local" {
		t.Fatalf("user name = %q, want local", rows[0].User.Name)
	}
}

func TestAdminRevokeTokenAudits(t *testing.T) {
	store := newTestStore(t)
	ownerSvc := newTestServiceWithRuntime(t, store, 100, "local", "local")
	created, err := ownerSvc.CreateToken(CreateTokenInput{
		Name:          "agent",
		Type:          "agent",
		Scopes:        []string{"task:read"},
		WorkspaceRefs: []string{"local"},
	})
	if err != nil {
		t.Fatal(err)
	}

	adminSvc, err := NewService(ServiceOptions{Store: store, Clock: FixedClock{NowUnix: 100}})
	if err != nil {
		t.Fatal(err)
	}

	if err := adminSvc.AdminRevokeToken(created.View.ID, "admin-ops"); err != nil {
		t.Fatalf("AdminRevokeToken() error = %v", err)
	}

	// 审计应含 admin.token.revoke + admin:true
	// admin service 无 workspace runtime，直接查 auditRepo 全量
	rows, err := adminSvc.auditRepo.List(storage.AuditListOptions{Limit: 50})
	if err != nil {
		t.Fatalf("auditRepo.List() error = %v", err)
	}
	found := false
	for _, row := range rows {
		if row.Action != "admin.token.revoke" {
			continue
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(row.PayloadJSON), &payload); err != nil {
			continue
		}
		if payload["admin"] == true && payload["admin_token_name"] == "admin-ops" {
			found = true
		}
	}
	if !found {
		t.Fatalf("audit missing admin.token.revoke with admin:true")
	}

	// 已吊销再吊销应被拒
	if err := adminSvc.AdminRevokeToken(created.View.ID, "admin-ops"); err == nil {
		t.Fatal("revoke already-revoked should fail")
	}
}

func TestAdminModifyTokenScopes(t *testing.T) {
	store := newTestStore(t)
	ownerSvc := newTestServiceWithRuntime(t, store, 100, "local", "local")
	created, err := ownerSvc.CreateToken(CreateTokenInput{
		Name:          "agent",
		Type:          "agent",
		Scopes:        []string{"task:read"},
		WorkspaceRefs: []string{"local"},
	})
	if err != nil {
		t.Fatal(err)
	}

	adminSvc, err := NewService(ServiceOptions{Store: store, Clock: FixedClock{NowUnix: 100}})
	if err != nil {
		t.Fatal(err)
	}

	view, err := adminSvc.AdminModifyToken(AdminModifyTokenInput{
		TokenID:        created.View.ID,
		Scopes:         &[]string{"task:read", "task:write"},
		AdminTokenName: "admin-ops",
	})
	if err != nil {
		t.Fatalf("AdminModifyToken() error = %v", err)
	}
	if len(view.Scopes) != 2 {
		t.Fatalf("scopes = %v, want 2", view.Scopes)
	}

	// 空 scope 被拒
	_, err = adminSvc.AdminModifyToken(AdminModifyTokenInput{
		TokenID: created.View.ID,
		Scopes:  &[]string{},
	})
	if err == nil {
		t.Fatal("empty scopes should be rejected")
	}
}

func TestAdminModifyTokenExpires(t *testing.T) {
	store := newTestStore(t)
	ownerSvc := newTestServiceWithRuntime(t, store, 100, "local", "local")
	created, err := ownerSvc.CreateToken(CreateTokenInput{
		Name:   "pat",
		Type:   "pat",
		Scopes: []string{"task:read"},
	})
	if err != nil {
		t.Fatal(err)
	}

	adminSvc, err := NewService(ServiceOptions{Store: store, Clock: FixedClock{NowUnix: 100}})
	if err != nil {
		t.Fatal(err)
	}

	// 设过期
	ttl := 720 * time.Hour
	view, err := adminSvc.AdminModifyToken(AdminModifyTokenInput{
		TokenID:   created.View.ID,
		ExpiresIn: &ttl,
	})
	if err != nil {
		t.Fatal(err)
	}
	if view.ExpiresAt == nil {
		t.Fatal("expires_at should be set")
	}

	// 清过期
	zero := time.Duration(0)
	view, err = adminSvc.AdminModifyToken(AdminModifyTokenInput{
		TokenID:   created.View.ID,
		ExpiresIn: &zero,
	})
	if err != nil {
		t.Fatal(err)
	}
	if view.ExpiresAt != nil {
		t.Fatalf("expires_at should be nil after clear, got %d", *view.ExpiresAt)
	}
}

func TestAdminModifyTokenRejectsRevoked(t *testing.T) {
	store := newTestStore(t)
	ownerSvc := newTestServiceWithRuntime(t, store, 100, "local", "local")
	created, err := ownerSvc.CreateToken(CreateTokenInput{
		Name:   "pat",
		Type:   "pat",
		Scopes: []string{"task:read"},
	})
	if err != nil {
		t.Fatal(err)
	}

	adminSvc, err := NewService(ServiceOptions{Store: store, Clock: FixedClock{NowUnix: 100}})
	if err != nil {
		t.Fatal(err)
	}
	if err := adminSvc.AdminRevokeToken(created.View.ID, "admin-ops"); err != nil {
		t.Fatal(err)
	}

	newName := "x"
	_, err = adminSvc.AdminModifyToken(AdminModifyTokenInput{
		TokenID: created.View.ID,
		Name:    &newName,
	})
	assertRuntimeCode(t, err, "token_revoked")
}

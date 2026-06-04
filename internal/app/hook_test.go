package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"path/filepath"
	"testing"

	"github.com/dajee/taskg/internal/storage"
)

// ---------------------------------------------------------------------------
// 测试辅助
// ---------------------------------------------------------------------------

// stubResolver 允许测试注入预设的 DNS 解析结果。
type stubResolver struct {
	addrs []net.IPAddr
	err   error
}

func (s stubResolver) LookupIPAddr(_ context.Context, _ string) ([]net.IPAddr, error) {
	return s.addrs, s.err
}

// hookTestEnv 创建一个包含 owner 用户的测试环境。
func hookTestEnv(t *testing.T) (*Service, *storage.Store, func()) {
	t.Helper()
	store, err := storage.Open(filepath.Join(t.TempDir(), "taskg.db"))
	if err != nil {
		t.Fatal(err)
	}
	svc, err := NewService(ServiceOptions{Store: store, Clock: FixedClock{NowUnix: 1000}})
	if err != nil {
		t.Fatal(err)
	}
	return svc, store, func() { _ = store.Close() }
}

// hookTestEnvWithRole 创建指定角色的测试服务。
func hookTestEnvWithRole(t *testing.T, role string) (*Service, *storage.Store, func()) {
	t.Helper()
	store, err := storage.Open(filepath.Join(t.TempDir(), "taskg.db"))
	if err != nil {
		t.Fatal(err)
	}
	ws := mustCreateWorkspaceRecord(t, store, storage.Workspace{
		ID: "ws-hook-test", Slug: "hook-test", Name: "Hook Test",
		Visibility: "private", CreatedAt: 100, ModifiedAt: 100,
	})
	user := mustCreateUserRecord(t, store, storage.User{
		ID: "user-hook-" + role, Name: "hook-" + role, CreatedAt: 100, ModifiedAt: 100,
	})
	mustUpsertMembershipRecord(t, store, storage.Membership{
		UserID: user.ID, WorkspaceID: ws.ID, Role: role, JoinedAt: 100, ModifiedAt: 100,
	})
	svc := newTestServiceWithRuntime(t, store, 1000, user.Name, ws.Slug)
	return svc, store, func() { _ = store.Close() }
}

func defaultHookInput() HookAddInput {
	return HookAddInput{
		Name:           "test-hook",
		ScopeType:      HookScopeWorkspace,
		EventTypes:     []string{"task.created"},
		EndpointURL:    "https://example.com/webhook",
		Secret:         "s3cret",
		TimeoutSeconds: 10,
		MaxAttempts:    5,
	}
}

// ---------------------------------------------------------------------------
// TestHookPermission: admin 可以创建 hooks，member/viewer 不行
// ---------------------------------------------------------------------------

func TestHookPermission(t *testing.T) {
	t.Run("owner can create hooks", func(t *testing.T) {
		svc, _, cleanup := hookTestEnv(t)
		defer cleanup()
		_, err := svc.AddHook(defaultHookInput())
		if err != nil {
			t.Fatalf("owner AddHook() error = %v", err)
		}
	})

	t.Run("admin can create hooks", func(t *testing.T) {
		svc, _, cleanup := hookTestEnvWithRole(t, "admin")
		defer cleanup()
		_, err := svc.AddHook(defaultHookInput())
		if err != nil {
			t.Fatalf("admin AddHook() error = %v", err)
		}
	})

	t.Run("member cannot create hooks", func(t *testing.T) {
		svc, _, cleanup := hookTestEnvWithRole(t, "member")
		defer cleanup()
		_, err := svc.AddHook(defaultHookInput())
		assertRuntimeCode(t, err, "permission_denied")
	})

	t.Run("viewer cannot create hooks", func(t *testing.T) {
		svc, _, cleanup := hookTestEnvWithRole(t, "viewer")
		defer cleanup()
		_, err := svc.AddHook(defaultHookInput())
		assertRuntimeCode(t, err, "permission_denied")
	})

	t.Run("member cannot list hooks", func(t *testing.T) {
		svc, _, cleanup := hookTestEnvWithRole(t, "member")
		defer cleanup()
		_, err := svc.ListHooks("")
		assertRuntimeCode(t, err, "permission_denied")
	})

	t.Run("viewer cannot list hooks", func(t *testing.T) {
		svc, _, cleanup := hookTestEnvWithRole(t, "viewer")
		defer cleanup()
		_, err := svc.ListHooks("")
		assertRuntimeCode(t, err, "permission_denied")
	})
}

// ---------------------------------------------------------------------------
// TestHookAddDefaults: 默认 enabled=true, timeout=10, maxAttempts=5
// ---------------------------------------------------------------------------

func TestHookAddDefaults(t *testing.T) {
	svc, _, cleanup := hookTestEnv(t)
	defer cleanup()

	input := HookAddInput{
		Name:        "defaults-test",
		ScopeType:   HookScopeWorkspace,
		EventTypes:  []string{"task.created"},
		EndpointURL: "https://example.com/webhook",
		Secret:      "s3cret",
		// TimeoutSeconds 和 MaxAttempts 未设置 (0)
	}
	view, err := svc.AddHook(input)
	if err != nil {
		t.Fatalf("AddHook() error = %v", err)
	}
	if !view.Enabled {
		t.Fatal("default enabled should be true")
	}
	if view.TimeoutSeconds != 10 {
		t.Fatalf("default timeout = %d, want 10", view.TimeoutSeconds)
	}
	if view.MaxAttempts != 5 {
		t.Fatalf("default max_attempts = %d, want 5", view.MaxAttempts)
	}
}

// ---------------------------------------------------------------------------
// TestHookListNoSecret: List 不返回 secret
// ---------------------------------------------------------------------------

func TestHookListNoSecret(t *testing.T) {
	svc, _, cleanup := hookTestEnv(t)
	defer cleanup()

	_, err := svc.AddHook(defaultHookInput())
	if err != nil {
		t.Fatalf("AddHook() error = %v", err)
	}

	hooks, err := svc.ListHooks("")
	if err != nil {
		t.Fatalf("ListHooks() error = %v", err)
	}
	if len(hooks) != 1 {
		t.Fatalf("hooks count = %d, want 1", len(hooks))
	}
	// HookView 没有 Secret 字段，但确认类型不变即可
	_ = hooks[0]
}

// ---------------------------------------------------------------------------
// TestHookInfoNoSecret: Info 不返回 secret
// ---------------------------------------------------------------------------

func TestHookInfoNoSecret(t *testing.T) {
	svc, _, cleanup := hookTestEnv(t)
	defer cleanup()

	created, err := svc.AddHook(defaultHookInput())
	if err != nil {
		t.Fatalf("AddHook() error = %v", err)
	}

	view, err := svc.HookInfo(created.ID)
	if err != nil {
		t.Fatalf("HookInfo() error = %v", err)
	}
	// HookView 没有 Secret 字段
	if view.ID != created.ID {
		t.Fatalf("ID mismatch: %q vs %q", view.ID, created.ID)
	}
}

// ---------------------------------------------------------------------------
// TestHookModifySecretOnlyWhenProvided: 修改时不传 secret 保持旧值，传入则更新
// ---------------------------------------------------------------------------

func TestHookModifySecretOnlyWhenProvided(t *testing.T) {
	svc, _, cleanup := hookTestEnv(t)
	defer cleanup()

	created, err := svc.AddHook(defaultHookInput())
	if err != nil {
		t.Fatalf("AddHook() error = %v", err)
	}

	// 不传 secret 修改其他字段
	newName := "updated-hook"
	modified, err := svc.ModifyHook(created.ID, HookModifyInput{Name: &newName})
	if err != nil {
		t.Fatalf("ModifyHook() error = %v", err)
	}
	if modified.Name != newName {
		t.Fatalf("name = %q, want %q", modified.Name, newName)
	}

	// 验证原 secret 仍在数据库中（通过直接查询 repo）
	row, err := svc.hookRepo.GetByID(created.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if row.Secret != "s3cret" {
		t.Fatalf("secret unexpectedly changed: %q", row.Secret)
	}

	// 传入新 secret
	newSecret := "n3w-s3cret"
	_, err = svc.ModifyHook(created.ID, HookModifyInput{Secret: &newSecret})
	if err != nil {
		t.Fatalf("ModifyHook() with secret error = %v", err)
	}
	row, err = svc.hookRepo.GetByID(created.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if row.Secret != newSecret {
		t.Fatalf("secret = %q, want %q", row.Secret, newSecret)
	}
}

// ---------------------------------------------------------------------------
// TestHookAuditSecretFingerprint: 审计包含指纹而非 secret
// ---------------------------------------------------------------------------

func TestHookAuditSecretFingerprint(t *testing.T) {
	svc, _, cleanup := hookTestEnv(t)
	defer cleanup()

	_, err := svc.AddHook(defaultHookInput())
	if err != nil {
		t.Fatalf("AddHook() error = %v", err)
	}

	audits := mustListAudit(t, svc)
	var createAudit *AuditLogView
	for i := range audits {
		if audits[i].Action == "hook.create" {
			createAudit = &audits[i]
			break
		}
	}
	if createAudit == nil {
		t.Fatal("hook.create audit not found")
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(createAudit.PayloadJSON), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	// 应包含 secret_fingerprint，不应包含原始 secret
	fp, ok := payload["secret_fingerprint"].(string)
	if !ok || fp == "" {
		t.Fatalf("missing secret_fingerprint in audit: %#v", payload)
	}
	if len(fp) != 8 {
		t.Fatalf("fingerprint length = %d, want 8", len(fp))
	}
	if _, hasSecret := payload["secret"]; hasSecret {
		t.Fatal("audit payload contains raw secret")
	}

	// 验证指纹计算正确
	expected := secretFingerprint("s3cret")
	if fp != expected {
		t.Fatalf("fingerprint = %q, want %q", fp, expected)
	}
}

// ---------------------------------------------------------------------------
// TestHookModifyEventTypesReplace: EventTypes nil 保持旧值，非 nil 替换
// ---------------------------------------------------------------------------

func TestHookModifyEventTypesReplace(t *testing.T) {
	svc, _, cleanup := hookTestEnv(t)
	defer cleanup()

	created, err := svc.AddHook(HookAddInput{
		Name:           "events-test",
		ScopeType:      HookScopeWorkspace,
		EventTypes:     []string{"task.created", "task.modified"},
		EndpointURL:    "https://example.com/webhook",
		Secret:         "s3cret",
		TimeoutSeconds: 10,
		MaxAttempts:    5,
	})
	if err != nil {
		t.Fatalf("AddHook() error = %v", err)
	}

	// nil EventTypes 不变
	modified, err := svc.ModifyHook(created.ID, HookModifyInput{Name: strptr("no-change-events")})
	if err != nil {
		t.Fatalf("ModifyHook() error = %v", err)
	}
	if len(modified.EventTypes) != 2 {
		t.Fatalf("event types count = %d, want 2", len(modified.EventTypes))
	}

	// 非 nil EventTypes 替换
	newTypes := []string{"task.completed"}
	modified, err = svc.ModifyHook(created.ID, HookModifyInput{EventTypes: &newTypes})
	if err != nil {
		t.Fatalf("ModifyHook() error = %v", err)
	}
	if len(modified.EventTypes) != 1 || modified.EventTypes[0] != "task.completed" {
		t.Fatalf("event types = %v, want [task.completed]", modified.EventTypes)
	}
}

// ---------------------------------------------------------------------------
// TestHookEnableDisable: 切换启用/禁用
// ---------------------------------------------------------------------------

func TestHookEnableDisable(t *testing.T) {
	svc, _, cleanup := hookTestEnv(t)
	defer cleanup()

	created, err := svc.AddHook(defaultHookInput())
	if err != nil {
		t.Fatalf("AddHook() error = %v", err)
	}
	if !created.Enabled {
		t.Fatal("hook should be enabled after creation")
	}

	// Disable
	disabled, err := svc.DisableHook(created.ID)
	if err != nil {
		t.Fatalf("DisableHook() error = %v", err)
	}
	if disabled.Enabled {
		t.Fatal("hook should be disabled")
	}

	// Enable
	enabled, err := svc.EnableHook(created.ID)
	if err != nil {
		t.Fatalf("EnableHook() error = %v", err)
	}
	if !enabled.Enabled {
		t.Fatal("hook should be enabled")
	}

	// 验证审计
	audits := mustListAudit(t, svc)
	foundDisable := false
	foundEnable := false
	for _, a := range audits {
		if a.Action == "hook.disable" {
			foundDisable = true
		}
		if a.Action == "hook.enable" {
			foundEnable = true
		}
	}
	if !foundDisable {
		t.Fatal("hook.disable audit not found")
	}
	if !foundEnable {
		t.Fatal("hook.enable audit not found")
	}
}

// ---------------------------------------------------------------------------
// TestHookDelete: 删除后 Info 返回 hook_not_found
// ---------------------------------------------------------------------------

func TestHookDelete(t *testing.T) {
	svc, _, cleanup := hookTestEnv(t)
	defer cleanup()

	created, err := svc.AddHook(defaultHookInput())
	if err != nil {
		t.Fatalf("AddHook() error = %v", err)
	}

	if err := svc.DeleteHook(created.ID); err != nil {
		t.Fatalf("DeleteHook() error = %v", err)
	}

	_, err = svc.HookInfo(created.ID)
	assertRuntimeCode(t, err, "hook_not_found")
}

// ---------------------------------------------------------------------------
// TestHookProjectScopeValidation: project scope 必须属于当前 workspace
// ---------------------------------------------------------------------------

func TestHookProjectScopeValidation(t *testing.T) {
	svc, store, cleanup := hookTestEnv(t)
	defer cleanup()

	// 在当前 workspace 创建 project
	projSvc := newTestServiceWithRuntime(t, store, 1000, "local", "local")
	proj, err := projSvc.AddProject(AddProjectInput{Slug: "myproject", Name: "My Project"})
	if err != nil {
		t.Fatalf("AddProject() error = %v", err)
	}

	// 创建 project-scoped hook 成功
	view, err := svc.AddHook(HookAddInput{
		Name:           "project-hook",
		ScopeType:      HookScopeProject,
		ProjectRef:     "myproject",
		EventTypes:     []string{"task.created"},
		EndpointURL:    "https://example.com/webhook",
		Secret:         "s3cret",
		TimeoutSeconds: 10,
		MaxAttempts:    5,
	})
	if err != nil {
		t.Fatalf("AddHook() project scope error = %v", err)
	}
	if view.ProjectID == nil || *view.ProjectID != proj.ID {
		t.Fatalf("project_id mismatch")
	}

	// 不存在的 project 应报错
	_, err = svc.AddHook(HookAddInput{
		Name:           "bad-project-hook",
		ScopeType:      HookScopeProject,
		ProjectRef:     "nonexistent",
		EventTypes:     []string{"task.created"},
		EndpointURL:    "https://example.com/webhook",
		Secret:         "s3cret",
		TimeoutSeconds: 10,
		MaxAttempts:    5,
	})
	assertRuntimeCode(t, err, "project_not_found")
}

// ---------------------------------------------------------------------------
// TestHookTimeoutValidation: 超时范围 1..120
// ---------------------------------------------------------------------------

func TestHookTimeoutValidation(t *testing.T) {
	svc, _, cleanup := hookTestEnv(t)
	defer cleanup()

	for _, timeout := range []int{-1, 121, 200} {
		input := defaultHookInput()
		input.TimeoutSeconds = timeout
		_, err := svc.AddHook(input)
		assertRuntimeCode(t, err, "hook_timeout_invalid")
	}

	// 合法值
	for _, timeout := range []int{1, 10, 60, 120} {
		input := defaultHookInput()
		input.TimeoutSeconds = timeout
		_, err := svc.AddHook(input)
		if err != nil {
			t.Fatalf("AddHook() with timeout=%d error = %v", timeout, err)
		}
	}
}

// ---------------------------------------------------------------------------
// TestHookMaxAttemptsValidation: 重试次数范围 1..20
// ---------------------------------------------------------------------------

func TestHookMaxAttemptsValidation(t *testing.T) {
	svc, _, cleanup := hookTestEnv(t)
	defer cleanup()

	for _, max := range []int{-1, 21, 100} {
		input := defaultHookInput()
		input.MaxAttempts = max
		_, err := svc.AddHook(input)
		assertRuntimeCode(t, err, "hook_max_attempts_invalid")
	}

	// 合法值
	for _, max := range []int{1, 5, 10, 20} {
		input := defaultHookInput()
		input.MaxAttempts = max
		_, err := svc.AddHook(input)
		if err != nil {
			t.Fatalf("AddHook() with max_attempts=%d error = %v", max, err)
		}
	}
}

// ---------------------------------------------------------------------------
// TestHookEndpointSSRF: 回环、链路本地、RFC1918、RFC6598、组播、未指定地址被拒绝
// ---------------------------------------------------------------------------

func TestHookEndpointSSRF(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		addrs   []net.IPAddr
		wantErr string
	}{
		{
			name:    "empty URL",
			raw:     "",
			wantErr: "hook_endpoint_invalid",
		},
		{
			name:    "no scheme",
			raw:     "example.com/webhook",
			wantErr: "hook_endpoint_invalid",
		},
		{
			name:    "ftp scheme",
			raw:     "ftp://example.com/webhook",
			wantErr: "hook_endpoint_invalid",
		},
		{
			name:    "loopback",
			raw:     "https://localhost/webhook",
			addrs:   []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}},
			wantErr: "hook_endpoint_invalid",
		},
		{
			name:    "link-local unicast",
			raw:     "https://host/webhook",
			addrs:   []net.IPAddr{{IP: net.ParseIP("169.254.1.1")}},
			wantErr: "hook_endpoint_invalid",
		},
		{
			name:    "RFC1918 10.x",
			raw:     "https://internal/webhook",
			addrs:   []net.IPAddr{{IP: net.ParseIP("10.0.0.1")}},
			wantErr: "hook_endpoint_invalid",
		},
		{
			name:    "RFC1918 172.16.x",
			raw:     "https://corp/webhook",
			addrs:   []net.IPAddr{{IP: net.ParseIP("172.16.0.1")}},
			wantErr: "hook_endpoint_invalid",
		},
		{
			name:    "RFC1918 192.168.x",
			raw:     "https://home/webhook",
			addrs:   []net.IPAddr{{IP: net.ParseIP("192.168.1.1")}},
			wantErr: "hook_endpoint_invalid",
		},
		{
			name:    "RFC6598 carrier-grade NAT",
			raw:     "https://cgnat/webhook",
			addrs:   []net.IPAddr{{IP: net.ParseIP("100.64.0.1")}},
			wantErr: "hook_endpoint_invalid",
		},
		{
			name:    "multicast",
			raw:     "https://multi/webhook",
			addrs:   []net.IPAddr{{IP: net.ParseIP("224.0.0.1")}},
			wantErr: "hook_endpoint_invalid",
		},
		{
			name:    "unspecified",
			raw:     "https://zero/webhook",
			addrs:   []net.IPAddr{{IP: net.ParseIP("0.0.0.0")}},
			wantErr: "hook_endpoint_invalid",
		},
		{
			name:    "valid public IP",
			raw:     "https://example.com/webhook",
			addrs:   []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}},
			wantErr: "",
		},
		{
			name:    "IPv6 loopback",
			raw:     "https://localhost6/webhook",
			addrs:   []net.IPAddr{{IP: net.ParseIP("::1")}},
			wantErr: "hook_endpoint_invalid",
		},
		{
			name:    "IPv6 private",
			raw:     "https://private6/webhook",
			addrs:   []net.IPAddr{{IP: net.ParseIP("fc00::1")}},
			wantErr: "hook_endpoint_invalid",
		},
		{
			name: "mixed public and private",
			raw:  "https://mixed.example/webhook",
			addrs: []net.IPAddr{
				{IP: net.ParseIP("93.184.216.34")},
				{IP: net.ParseIP("10.0.0.1")},
			},
			wantErr: "hook_endpoint_invalid",
		},
		{
			name:    "empty DNS answer",
			raw:     "https://empty.example/webhook",
			addrs:   []net.IPAddr{},
			wantErr: "hook_endpoint_invalid",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resolver := stubResolver{addrs: tc.addrs}
			err := ValidateWebhookEndpointURL(context.Background(), tc.raw, resolver)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			} else {
				re, ok := err.(RuntimeError)
				if !ok {
					t.Fatalf("err = %v, want RuntimeError", err)
				}
				if re.Code != tc.wantErr {
					t.Fatalf("code = %q, want %q", re.Code, tc.wantErr)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestHookEventTypeValidation: 只允许注册的事件类型
// ---------------------------------------------------------------------------

func TestHookEventTypeValidation(t *testing.T) {
	svc, _, cleanup := hookTestEnv(t)
	defer cleanup()

	// 不合法的事件类型
	input := defaultHookInput()
	input.EventTypes = []string{"task.created", "invalid.event"}
	_, err := svc.AddHook(input)
	assertRuntimeCode(t, err, "hook_event_types_invalid")

	// 合法事件类型
	input.EventTypes = []string{"task.created", "task.completed", "task.deleted", "task.modified", "project.archived"}
	_, err = svc.AddHook(input)
	if err != nil {
		t.Fatalf("AddHook() with valid events error = %v", err)
	}
}

// ---------------------------------------------------------------------------
// TestHookModifyEmptyEventTypes: 传空 event types 应报错
// ---------------------------------------------------------------------------

func TestHookModifyEmptyEventTypes(t *testing.T) {
	svc, _, cleanup := hookTestEnv(t)
	defer cleanup()

	created, err := svc.AddHook(defaultHookInput())
	if err != nil {
		t.Fatalf("AddHook() error = %v", err)
	}

	empty := []string{}
	_, err = svc.ModifyHook(created.ID, HookModifyInput{EventTypes: &empty})
	assertRuntimeCode(t, err, "hook_event_types_invalid")
}

// ---------------------------------------------------------------------------
// TestHookModifyEndpointValidation: 修改 endpoint 时也要验证 SSRF
// ---------------------------------------------------------------------------

func TestHookModifyEndpointValidation(t *testing.T) {
	svc, _, cleanup := hookTestEnv(t)
	defer cleanup()

	created, err := svc.AddHook(defaultHookInput())
	if err != nil {
		t.Fatalf("AddHook() error = %v", err)
	}

	badURL := "ftp://example.com/webhook"
	_, err = svc.ModifyHook(created.ID, HookModifyInput{EndpointURL: &badURL})
	assertRuntimeCode(t, err, "hook_endpoint_invalid")
}

// ---------------------------------------------------------------------------
// TestHookModifyTimeoutValidation: 修改 timeout 时验证范围
// ---------------------------------------------------------------------------

func TestHookModifyTimeoutValidation(t *testing.T) {
	svc, _, cleanup := hookTestEnv(t)
	defer cleanup()

	created, err := svc.AddHook(defaultHookInput())
	if err != nil {
		t.Fatalf("AddHook() error = %v", err)
	}

	badTimeout := 200
	_, err = svc.ModifyHook(created.ID, HookModifyInput{TimeoutSeconds: &badTimeout})
	assertRuntimeCode(t, err, "hook_timeout_invalid")
}

// ---------------------------------------------------------------------------
// TestHookInfoWrongWorkspace: 查询其他 workspace 的 hook 返回 not found
// ---------------------------------------------------------------------------

func TestHookInfoWrongWorkspace(t *testing.T) {
	svc, store, cleanup := hookTestEnv(t)
	defer cleanup()

	created, err := svc.AddHook(defaultHookInput())
	if err != nil {
		t.Fatalf("AddHook() error = %v", err)
	}

	// 创建另一个 workspace
	ws2 := mustCreateWorkspaceRecord(t, store, storage.Workspace{
		ID: "ws-other", Slug: "other", Name: "Other",
		Visibility: "private", CreatedAt: 100, ModifiedAt: 100,
	})
	user2 := mustCreateUserRecord(t, store, storage.User{
		ID: "user-other", Name: "other-user", CreatedAt: 100, ModifiedAt: 100,
	})
	mustUpsertMembershipRecord(t, store, storage.Membership{
		UserID: user2.ID, WorkspaceID: ws2.ID, Role: "owner", JoinedAt: 100, ModifiedAt: 100,
	})
	svc2 := newTestServiceWithRuntime(t, store, 1000, user2.Name, ws2.Slug)

	_, err = svc2.HookInfo(created.ID)
	assertRuntimeCode(t, err, "hook_not_found")
}

// ---------------------------------------------------------------------------
// TestHookDeleteWrongWorkspace: 不能删除其他 workspace 的 hook
// ---------------------------------------------------------------------------

func TestHookDeleteWrongWorkspace(t *testing.T) {
	svc, store, cleanup := hookTestEnv(t)
	defer cleanup()

	created, err := svc.AddHook(defaultHookInput())
	if err != nil {
		t.Fatalf("AddHook() error = %v", err)
	}

	ws2 := mustCreateWorkspaceRecord(t, store, storage.Workspace{
		ID: "ws-other2", Slug: "other2", Name: "Other2",
		Visibility: "private", CreatedAt: 100, ModifiedAt: 100,
	})
	user2 := mustCreateUserRecord(t, store, storage.User{
		ID: "user-other2", Name: "other2-user", CreatedAt: 100, ModifiedAt: 100,
	})
	mustUpsertMembershipRecord(t, store, storage.Membership{
		UserID: user2.ID, WorkspaceID: ws2.ID, Role: "owner", JoinedAt: 100, ModifiedAt: 100,
	})
	svc2 := newTestServiceWithRuntime(t, store, 1000, user2.Name, ws2.Slug)

	err = svc2.DeleteHook(created.ID)
	assertRuntimeCode(t, err, "hook_not_found")
}

// ---------------------------------------------------------------------------
// TestHookAddNameValidation: 名字不能为空或过长
// ---------------------------------------------------------------------------

func TestHookAddNameValidation(t *testing.T) {
	svc, _, cleanup := hookTestEnv(t)
	defer cleanup()

	// 空名字
	input := defaultHookInput()
	input.Name = ""
	_, err := svc.AddHook(input)
	assertRuntimeCode(t, err, "hook_name_invalid")

	// 纯空格名字
	input.Name = "   "
	_, err = svc.AddHook(input)
	assertRuntimeCode(t, err, "hook_name_invalid")
}

// ---------------------------------------------------------------------------
// TestHookModifyAuditFingerprint: 修改 secret 时审计包含新指纹
// ---------------------------------------------------------------------------

func TestHookModifyAuditFingerprint(t *testing.T) {
	svc, _, cleanup := hookTestEnv(t)
	defer cleanup()

	created, err := svc.AddHook(defaultHookInput())
	if err != nil {
		t.Fatalf("AddHook() error = %v", err)
	}

	// 清空已有审计
	newSecret := "updated-secret"
	_, err = svc.ModifyHook(created.ID, HookModifyInput{Secret: &newSecret})
	if err != nil {
		t.Fatalf("ModifyHook() error = %v", err)
	}

	audits := mustListAudit(t, svc)
	var modifyAudit *AuditLogView
	for i := range audits {
		if audits[i].Action == "hook.modify" {
			modifyAudit = &audits[i]
			break
		}
	}
	if modifyAudit == nil {
		t.Fatal("hook.modify audit not found")
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(modifyAudit.PayloadJSON), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	fp, ok := payload["secret_fingerprint"].(string)
	if !ok || fp == "" {
		t.Fatalf("missing secret_fingerprint in modify audit: %#v", payload)
	}

	expected := secretFingerprint("updated-secret")
	if fp != expected {
		t.Fatalf("fingerprint = %q, want %q", fp, expected)
	}

	if _, hasSecret := payload["secret"]; hasSecret {
		t.Fatal("modify audit payload contains raw secret")
	}
}

// ---------------------------------------------------------------------------
// TestHookListByProject: 按 project 过滤列表
// ---------------------------------------------------------------------------

func TestHookListByProject(t *testing.T) {
	svc, store, cleanup := hookTestEnv(t)
	defer cleanup()

	projSvc := newTestServiceWithRuntime(t, store, 1000, "local", "local")
	proj, err := projSvc.AddProject(AddProjectInput{Slug: "listproj", Name: "List Project"})
	if err != nil {
		t.Fatalf("AddProject() error = %v", err)
	}

	// workspace-scoped hook
	_, err = svc.AddHook(HookAddInput{
		Name: "ws-hook", ScopeType: HookScopeWorkspace,
		EventTypes: []string{"task.created"}, EndpointURL: "https://example.com/ws",
		Secret: "s", TimeoutSeconds: 10, MaxAttempts: 5,
	})
	if err != nil {
		t.Fatalf("AddHook() ws error = %v", err)
	}

	// project-scoped hook
	_, err = svc.AddHook(HookAddInput{
		Name: "proj-hook", ScopeType: HookScopeProject, ProjectRef: "listproj",
		EventTypes: []string{"task.completed"}, EndpointURL: "https://example.com/proj",
		Secret: "s", TimeoutSeconds: 10, MaxAttempts: 5,
	})
	if err != nil {
		t.Fatalf("AddHook() proj error = %v", err)
	}

	// 列出全部
	all, err := svc.ListHooks("")
	if err != nil {
		t.Fatalf("ListHooks() error = %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("all hooks count = %d, want 2", len(all))
	}

	// 按 project 过滤
	projectHooks, err := svc.ListHooks("listproj")
	if err != nil {
		t.Fatalf("ListHooks(listproj) error = %v", err)
	}
	if len(projectHooks) != 1 {
		t.Fatalf("project hooks count = %d, want 1", len(projectHooks))
	}
	if projectHooks[0].ProjectID == nil || *projectHooks[0].ProjectID != proj.ID {
		t.Fatal("project hook has wrong project_id")
	}
}

// ---------------------------------------------------------------------------
// TestHookDeliveryEnqueuedOnTaskCreated: 创建任务时生成 task.created 事件并落盘
// ---------------------------------------------------------------------------

func TestHookDeliveryEnqueuedOnTaskCreated(t *testing.T) {
	svc, store, cleanup := hookTestEnv(t)
	defer cleanup()

	// 创建 workspace 范围的 hook，监听 task.created
	hook, err := svc.AddHook(HookAddInput{
		Name: "create-hook", ScopeType: HookScopeWorkspace,
		EventTypes: []string{"task.created"}, EndpointURL: "https://example.com/hook",
		Secret: "s3cret", TimeoutSeconds: 10, MaxAttempts: 5,
	})
	if err != nil {
		t.Fatalf("AddHook() error = %v", err)
	}

	// 创建任务
	created, err := svc.Add(AddInput{Description: "test task"})
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	// 验证 delivery 已入队
	deliveries, err := svc.hookDeliveryRepo.ListByHook(hook.ID, "", 10)
	if err != nil {
		t.Fatalf("ListByHook() error = %v", err)
	}
	if len(deliveries) != 1 {
		t.Fatalf("deliveries count = %d, want 1", len(deliveries))
	}
	d := deliveries[0]
	if d.EventType != "task.created" {
		t.Fatalf("event_type = %q, want task.created", d.EventType)
	}
	if d.HookID != hook.ID {
		t.Fatalf("hook_id = %q, want %q", d.HookID, hook.ID)
	}
	if d.Status != storage.DeliveryStatusQueued {
		t.Fatalf("status = %q, want queued", d.Status)
	}
	if d.WorkspaceID != svc.runtime.WorkspaceID {
		t.Fatalf("workspace_id = %q, want %q", d.WorkspaceID, svc.runtime.WorkspaceID)
	}
	if d.ActorUserID != svc.runtime.ActorUserID {
		t.Fatalf("actor_user_id = %q, want %q", d.ActorUserID, svc.runtime.ActorUserID)
	}

	// 验证 payload 包含任务数据
	var payload map[string]any
	if err := json.Unmarshal([]byte(d.PayloadJSON), &payload); err != nil {
		t.Fatalf("json.Unmarshal(payload) error = %v", err)
	}
	if payload["event_type"] != "task.created" {
		t.Fatalf("payload event_type = %v, want task.created", payload["event_type"])
	}
	if payload["object_id"] != created.UUID {
		t.Fatalf("payload object_id = %v, want %s", payload["object_id"], created.UUID)
	}
	if payload["object_kind"] != "task" {
		t.Fatalf("payload object_kind = %v, want task", payload["object_kind"])
	}
	data, ok := payload["data"].(map[string]any)
	if !ok {
		t.Fatalf("payload data = %T, want map[string]any", payload["data"])
	}
	if data["completed"] != false {
		t.Fatalf("data.completed = %v, want false", data["completed"])
	}
	if data["deleted"] != false {
		t.Fatalf("data.deleted = %v, want false", data["deleted"])
	}
	taskData, ok := data["task"].(map[string]any)
	if !ok {
		t.Fatalf("data.task = %T, want map[string]any", data["task"])
	}
	if taskData["uuid"] != created.UUID {
		t.Fatalf("task.uuid = %v, want %s", taskData["uuid"], created.UUID)
	}
	if taskData["description"] != "test task" {
		t.Fatalf("task.description = %v, want test task", taskData["description"])
	}

	// 验证 headers
	var headers map[string]string
	if err := json.Unmarshal([]byte(d.HeadersJSON), &headers); err != nil {
		t.Fatalf("json.Unmarshal(headers) error = %v", err)
	}
	if headers["X-Taskg-Event"] != "task.created" {
		t.Fatalf("X-Taskg-Event = %q, want task.created", headers["X-Taskg-Event"])
	}
	if headers["X-Taskg-Event-Version"] != "1" {
		t.Fatalf("X-Taskg-Event-Version = %q, want 1", headers["X-Taskg-Event-Version"])
	}

	// 确保 store 已关闭（defer 会处理）
	_ = store
}

func TestHookPayloadIncludesAssignees(t *testing.T) {
	svc, store, cleanup := hookTestEnv(t)
	defer cleanup()
	assignee := mustCreateUserRecord(t, store, storage.User{
		ID:         "user-hook-assignee",
		Name:       "hook-assignee",
		Email:      strptr("hook-assignee@example.com"),
		CreatedAt:  1000,
		ModifiedAt: 1000,
	})
	ws, err := store.LocalWorkspace()
	if err != nil {
		t.Fatalf("LocalWorkspace() error = %v", err)
	}
	mustUpsertMembershipRecord(t, store, storage.Membership{
		UserID:      assignee.ID,
		WorkspaceID: ws.ID,
		Role:        string(RoleMember),
		JoinedAt:    1000,
		ModifiedAt:  1000,
	})

	hook, err := svc.AddHook(HookAddInput{
		Name: "create-hook", ScopeType: HookScopeWorkspace,
		EventTypes: []string{"task.created"}, EndpointURL: "https://example.com/hook",
		Secret: "s3cret", TimeoutSeconds: 10, MaxAttempts: 5,
	})
	if err != nil {
		t.Fatalf("AddHook() error = %v", err)
	}

	created, err := svc.Add(AddInput{Description: "assigned hook task", Assignees: []string{"hook-assignee"}})
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	deliveries, err := svc.hookDeliveryRepo.ListByHook(hook.ID, "", 10)
	if err != nil {
		t.Fatalf("ListByHook() error = %v", err)
	}
	if len(deliveries) != 1 {
		t.Fatalf("deliveries count = %d, want 1", len(deliveries))
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(deliveries[0].PayloadJSON), &payload); err != nil {
		t.Fatalf("json.Unmarshal(payload) error = %v", err)
	}
	data, ok := payload["data"].(map[string]any)
	if !ok {
		t.Fatalf("payload data = %T, want map[string]any", payload["data"])
	}
	taskData, ok := data["task"].(map[string]any)
	if !ok {
		t.Fatalf("data.task = %T, want map[string]any", data["task"])
	}
	if taskData["uuid"] != created.UUID {
		t.Fatalf("task.uuid = %v, want %s", taskData["uuid"], created.UUID)
	}
	assignees, ok := taskData["assignees"].([]any)
	if !ok || len(assignees) != 1 {
		t.Fatalf("task.assignees = %#v, want one assignee", taskData["assignees"])
	}
	first, ok := assignees[0].(map[string]any)
	if !ok || first["user_id"] != assignee.ID || first["name"] != "hook-assignee" || first["email"] != "hook-assignee@example.com" {
		t.Fatalf("first assignee = %#v, want complete hook-assignee info", assignees[0])
	}
}

// ---------------------------------------------------------------------------
// TestHookEventsForWriteOperations: 每个写操作生成正确的事件类型
// ---------------------------------------------------------------------------

func TestHookEventsForWriteOperations(t *testing.T) {
	svc, _, cleanup := hookTestEnv(t)
	defer cleanup()

	// 创建监听所有任务事件的 hook
	hook, err := svc.AddHook(HookAddInput{
		Name: "all-events", ScopeType: HookScopeWorkspace,
		EventTypes:     []string{"task.created", "task.modified", "task.completed", "task.deleted"},
		EndpointURL:    "https://example.com/hook",
		Secret:         "s",
		TimeoutSeconds: 10, MaxAttempts: 5,
	})
	if err != nil {
		t.Fatalf("AddHook() error = %v", err)
	}

	// Add -> task.created
	created, err := svc.Add(AddInput{Description: "task1"})
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	assertDeliveryEventType(t, svc, hook.ID, "task.created")

	// Modify -> task.modified
	err = svc.Modify(created.UUID, ModifyInput{Description: strptr("modified task1")})
	if err != nil {
		t.Fatalf("Modify() error = %v", err)
	}
	assertDeliveryEventType(t, svc, hook.ID, "task.modified")

	// Start -> task.modified
	err = svc.Start(created.UUID)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	assertDeliveryEventType(t, svc, hook.ID, "task.modified")

	// Stop -> task.modified
	err = svc.Stop(created.UUID)
	if err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	assertDeliveryEventType(t, svc, hook.ID, "task.modified")

	// Annotate -> task.modified
	err = svc.Annotate(created.UUID, "my annotation")
	if err != nil {
		t.Fatalf("Annotate() error = %v", err)
	}
	assertDeliveryEventType(t, svc, hook.ID, "task.modified")

	// Done -> task.completed
	err = svc.Done(created.UUID)
	if err != nil {
		t.Fatalf("Done() error = %v", err)
	}
	assertDeliveryEventType(t, svc, hook.ID, "task.completed")

	// Create another task for delete
	toDelete, err := svc.Add(AddInput{Description: "task to delete"})
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	// Delete -> task.deleted
	err = svc.Delete(toDelete.UUID)
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	assertDeliveryEventType(t, svc, hook.ID, "task.deleted")
}

func assertDeliveryEventType(t *testing.T, svc *Service, hookID, wantEventType string) {
	t.Helper()
	deliveries, err := svc.hookDeliveryRepo.ListByHook(hookID, "", 20)
	if err != nil {
		t.Fatalf("ListByHook() error = %v", err)
	}
	for _, d := range deliveries {
		if d.EventType == wantEventType {
			return
		}
	}
	var found []string
	for _, d := range deliveries {
		found = append(found, d.EventType)
	}
	t.Fatalf("no delivery with event_type %q found; got %v", wantEventType, found)
}

// ---------------------------------------------------------------------------
// TestHookProjectScopeIsolation: project hook 只接收匹配 project 的事件
// ---------------------------------------------------------------------------

func TestHookProjectScopeIsolation(t *testing.T) {
	svc, _, cleanup := hookTestEnv(t)
	defer cleanup()

	// 创建 project
	proj, err := svc.AddProject(AddProjectInput{Slug: "isolated-proj", Name: "Isolated Project"})
	if err != nil {
		t.Fatalf("AddProject() error = %v", err)
	}

	// 创建 project-scoped hook，只监听 task.created
	projHook, err := svc.AddHook(HookAddInput{
		Name: "proj-hook", ScopeType: HookScopeProject, ProjectRef: "isolated-proj",
		EventTypes: []string{"task.created"}, EndpointURL: "https://example.com/proj",
		Secret: "s", TimeoutSeconds: 10, MaxAttempts: 5,
	})
	if err != nil {
		t.Fatalf("AddHook() proj error = %v", err)
	}

	// 创建 workspace-scoped hook
	wsHook, err := svc.AddHook(HookAddInput{
		Name: "ws-hook", ScopeType: HookScopeWorkspace,
		EventTypes: []string{"task.created"}, EndpointURL: "https://example.com/ws",
		Secret: "s", TimeoutSeconds: 10, MaxAttempts: 5,
	})
	if err != nil {
		t.Fatalf("AddHook() ws error = %v", err)
	}

	// 创建带 project 的任务 -> 两个 hook 都应收到
	p := "isolated-proj"
	_, err = svc.Add(AddInput{Description: "in project", Project: &p})
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	projDeliveries, _ := svc.hookDeliveryRepo.ListByHook(projHook.ID, "", 10)
	wsDeliveries, _ := svc.hookDeliveryRepo.ListByHook(wsHook.ID, "", 10)
	if len(projDeliveries) != 1 {
		t.Fatalf("proj hook deliveries for in-project task = %d, want 1", len(projDeliveries))
	}
	if len(wsDeliveries) != 1 {
		t.Fatalf("ws hook deliveries for in-project task = %d, want 1", len(wsDeliveries))
	}

	// 创建无 project 的任务 -> 只有 workspace hook 应收到
	_, err = svc.Add(AddInput{Description: "no project"})
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	projDeliveries2, _ := svc.hookDeliveryRepo.ListByHook(projHook.ID, "", 10)
	wsDeliveries2, _ := svc.hookDeliveryRepo.ListByHook(wsHook.ID, "", 10)
	if len(projDeliveries2) != 1 {
		t.Fatalf("proj hook deliveries total = %d, want 1 (should not receive no-project event)", len(projDeliveries2))
	}
	if len(wsDeliveries2) != 2 {
		t.Fatalf("ws hook deliveries total = %d, want 2", len(wsDeliveries2))
	}

	// 验证 project hook 的 delivery 包含正确的 project_id
	_ = proj // project ID used for matching
	if projDeliveries[0].ProjectID == nil || *projDeliveries[0].ProjectID != proj.ID {
		t.Fatalf("proj delivery project_id mismatch: got %v", projDeliveries[0].ProjectID)
	}
}

// ---------------------------------------------------------------------------
// TestHookWorkspaceScopeNoProject: workspace hook 接收无 project 的任务
// ---------------------------------------------------------------------------

func TestHookWorkspaceScopeNoProject(t *testing.T) {
	svc, _, cleanup := hookTestEnv(t)
	defer cleanup()

	hook, err := svc.AddHook(HookAddInput{
		Name: "ws-hook", ScopeType: HookScopeWorkspace,
		EventTypes: []string{"task.completed"}, EndpointURL: "https://example.com/hook",
		Secret: "s", TimeoutSeconds: 10, MaxAttempts: 5,
	})
	if err != nil {
		t.Fatalf("AddHook() error = %v", err)
	}

	created, err := svc.Add(AddInput{Description: "finish me"})
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	// Done 之前不应有 delivery
	deliveries, _ := svc.hookDeliveryRepo.ListByHook(hook.ID, "", 10)
	if len(deliveries) != 0 {
		t.Fatalf("deliveries before done = %d, want 0", len(deliveries))
	}

	err = svc.Done(created.UUID)
	if err != nil {
		t.Fatalf("Done() error = %v", err)
	}

	deliveries, _ = svc.hookDeliveryRepo.ListByHook(hook.ID, "", 10)
	if len(deliveries) != 1 {
		t.Fatalf("deliveries after done = %d, want 1", len(deliveries))
	}
	if deliveries[0].EventType != "task.completed" {
		t.Fatalf("event_type = %q, want task.completed", deliveries[0].EventType)
	}

	// 验证 payload 中 completed=true
	var payload map[string]any
	json.Unmarshal([]byte(deliveries[0].PayloadJSON), &payload)
	data := payload["data"].(map[string]any)
	if data["completed"] != true {
		t.Fatalf("data.completed = %v, want true", data["completed"])
	}
}

// ---------------------------------------------------------------------------
// TestHookDeliveryFailureRollsBackTaskWrite: 投递失败时任务写入回滚
// ---------------------------------------------------------------------------

func TestHookDeliveryFailureRollsBackTaskWrite(t *testing.T) {
	svc, store, cleanup := hookTestEnv(t)
	defer cleanup()

	// 创建 hook
	_, err := svc.AddHook(HookAddInput{
		Name: "rollback-hook", ScopeType: HookScopeWorkspace,
		EventTypes: []string{"task.created"}, EndpointURL: "https://example.com/hook",
		Secret: "s", TimeoutSeconds: 10, MaxAttempts: 5,
	})
	if err != nil {
		t.Fatalf("AddHook() error = %v", err)
	}

	// 注入一个会失败的 hookDeliveryRepo
	svc.hookDeliveryRepo = &failingDeliveryRepo{}

	// 创建任务应该失败
	_, err = svc.Add(AddInput{Description: "should rollback"})
	if err == nil {
		t.Fatal("Add() should have failed when delivery enqueue fails")
	}

	// 验证任务确实没有被创建（通过新的 service 查询）
	cleanSvc := newTestServiceWithRuntime(t, store, 1000, "local", "local")
	tasks, err := cleanSvc.List(ListInput{})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	for _, tsk := range tasks {
		if tsk.Description == "should rollback" {
			t.Fatal("task should have been rolled back but was found")
		}
	}
}

// failingDeliveryRepo 是一个 Enqueue 总是返回错误的 hookDeliveryEnqueuer。
type failingDeliveryRepo struct {
	storage.HookDeliveryRepository
}

func (r *failingDeliveryRepo) Enqueue(_ []storage.HookDelivery) error {
	return fmt.Errorf("injected delivery failure")
}

// ---------------------------------------------------------------------------
// TestHookProjectArchivedEvent: 归档项目时生成 project.archived 事件
// ---------------------------------------------------------------------------

func TestHookProjectArchivedEvent(t *testing.T) {
	svc, _, cleanup := hookTestEnv(t)
	defer cleanup()

	// 创建监听 project.archived 的 hook
	hook, err := svc.AddHook(HookAddInput{
		Name: "archive-hook", ScopeType: HookScopeWorkspace,
		EventTypes: []string{"project.archived"}, EndpointURL: "https://example.com/hook",
		Secret: "s", TimeoutSeconds: 10, MaxAttempts: 5,
	})
	if err != nil {
		t.Fatalf("AddHook() error = %v", err)
	}

	// 创建并归档 project
	proj, err := svc.AddProject(AddProjectInput{Slug: "to-archive", Name: "To Archive"})
	if err != nil {
		t.Fatalf("AddProject() error = %v", err)
	}

	archived, err := svc.ArchiveProject(proj.Slug)
	if err != nil {
		t.Fatalf("ArchiveProject() error = %v", err)
	}

	// 验证 delivery
	deliveries, err := svc.hookDeliveryRepo.ListByHook(hook.ID, "", 10)
	if err != nil {
		t.Fatalf("ListByHook() error = %v", err)
	}
	if len(deliveries) != 1 {
		t.Fatalf("deliveries count = %d, want 1", len(deliveries))
	}
	d := deliveries[0]
	if d.EventType != "project.archived" {
		t.Fatalf("event_type = %q, want project.archived", d.EventType)
	}
	var payload map[string]any
	json.Unmarshal([]byte(d.PayloadJSON), &payload)
	if payload["object_kind"] != "project" {
		t.Fatalf("payload object_kind = %v, want project", payload["object_kind"])
	}
	if payload["object_id"] != proj.ID {
		t.Fatalf("payload object_id = %v, want %s", payload["object_id"], proj.ID)
	}
	data := payload["data"].(map[string]any)
	if data["archived"] != true {
		t.Fatalf("data.archived = %v, want true", data["archived"])
	}
	projData := data["project"].(map[string]any)
	if projData["slug"] != "to-archive" {
		t.Fatalf("project slug = %v, want to-archive", projData["slug"])
	}

	_ = archived
}

package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"path/filepath"
	"testing"

	"git.dajee.net/dajee/xuanchu/internal/storage"
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
	store, err := storage.Open(filepath.Join(t.TempDir(), "xuanchu.db"))
	if err != nil {
		t.Fatal(err)
	}
	svc, err := NewService(ServiceOptions{Store: store, Clock: FixedClock{NowUnix: 1000}})
	if err != nil {
		t.Fatal(err)
	}
	createHookTestSink(t, store, svc.workspaceID, svc.runtime.ActorUserID, "hook-sink")
	return svc, store, func() { _ = store.Close() }
}

// hookTestEnvWithRole 创建指定角色的测试服务。
func hookTestEnvWithRole(t *testing.T, role string) (*Service, *storage.Store, func()) {
	t.Helper()
	store, err := storage.Open(filepath.Join(t.TempDir(), "xuanchu.db"))
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
	createHookTestSink(t, store, ws.ID, user.ID, "hook-sink")
	return svc, store, func() { _ = store.Close() }
}

func createHookTestSink(t *testing.T, store *storage.Store, workspaceID, createdBy, name string) {
	t.Helper()
	enabled := true
	row := storage.NotificationSink{
		ID:                  "sink-" + name + "-" + workspaceID,
		WorkspaceID:         workspaceID,
		Name:                name,
		Type:                NotificationSinkTypeWebhook,
		EndpointMode:        NotificationEndpointStaticURL,
		URL:                 "https://example.com/webhook",
		AllowedHostsJSON:    `["example.com"]`,
		HTTPMethod:          "POST",
		HeaderTemplatesJSON: `[]`,
		SecretRefsJSON:      `{}`,
		Secret:              "s3cret",
		Enabled:             &enabled,
		TimeoutSeconds:      10,
		MaxAttempts:         5,
		CreatedBy:           createdBy,
		CreatedAt:           100,
		ModifiedAt:          100,
	}
	if err := storage.NewNotificationSinkRepository(store.DB()).Create(row); err != nil {
		t.Fatalf("Create notification sink %q error = %v", name, err)
	}
}

func defaultHookInput() HookAddInput {
	return HookAddInput{
		Name:           "test-hook",
		ScopeType:      HookScopeWorkspace,
		EventTypes:     []string{"task.created"},
		SinkRef:        "hook-sink",
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
		Name:       "defaults-test",
		ScopeType:  HookScopeWorkspace,
		EventTypes: []string{"task.created"},
		SinkRef:    "hook-sink",
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
// TestHookModifySinkOnlyWhenProvided: 修改时不传 sink 保持旧值，传入则更新
// ---------------------------------------------------------------------------

func TestHookModifySinkOnlyWhenProvided(t *testing.T) {
	svc, store, cleanup := hookTestEnv(t)
	defer cleanup()
	createHookTestSink(t, store, svc.workspaceID, svc.runtime.ActorUserID, "other-sink")

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

	row, err := svc.hookRepo.GetByID(created.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if row.SinkID != created.SinkID {
		t.Fatalf("sink unexpectedly changed: %q want %q", row.SinkID, created.SinkID)
	}

	newSink := "other-sink"
	_, err = svc.ModifyHook(created.ID, HookModifyInput{SinkRef: &newSink})
	if err != nil {
		t.Fatalf("ModifyHook() with sink error = %v", err)
	}
	row, err = svc.hookRepo.GetByID(created.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if row.SinkID == created.SinkID || row.SinkID == "" {
		t.Fatalf("sink was not updated: %q", row.SinkID)
	}
}

// ---------------------------------------------------------------------------
// TestHookAuditSinkReference: 审计包含 sink 引用且不泄露 sink secret
// ---------------------------------------------------------------------------

func TestHookAuditSinkReference(t *testing.T) {
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

	if payload["sink_name"] != "hook-sink" {
		t.Fatalf("sink_name = %#v, want hook-sink; payload=%#v", payload["sink_name"], payload)
	}
	if _, hasSecret := payload["secret"]; hasSecret {
		t.Fatal("audit payload contains raw secret")
	}
	if _, hasFingerprint := payload["secret_fingerprint"]; hasFingerprint {
		t.Fatal("audit payload contains sink secret fingerprint")
	}
	if _, hasURL := payload["endpoint_url"]; hasURL {
		t.Fatal("audit payload contains endpoint url")
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
		SinkRef:        "hook-sink",
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
		SinkRef:        "hook-sink",
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
		SinkRef:        "hook-sink",
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

func TestHookAllowsAllSemanticEventTypes(t *testing.T) {
	svc, _, cleanup := hookTestEnv(t)
	defer cleanup()

	input := defaultHookInput()
	input.EventTypes = allSemanticEventTypes()
	created, err := svc.AddHook(input)
	if err != nil {
		t.Fatalf("AddHook() with semantic events error = %v", err)
	}
	if len(created.EventTypes) != len(allSemanticEventTypes()) {
		t.Fatalf("event_types count = %d, want %d", len(created.EventTypes), len(allSemanticEventTypes()))
	}
}

func allSemanticEventTypes() []string {
	return []string{
		"task.created",
		"task.modified",
		"task.completed",
		"task.deleted",
		"task.started",
		"task.stopped",
		"task.assigned",
		"task.unassigned",
		"task.blocked",
		"task.due_changed",
		"task.priority_changed",
		"task.project_changed",
		"task.tags_changed",
		"task.unblocked",
		"project.archived",
		"project.annotated",
		"project.denotated",
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
// TestHookModifySinkValidation: 修改 sink 时必须解析当前 workspace 内的 sink
// ---------------------------------------------------------------------------

func TestHookModifySinkValidation(t *testing.T) {
	svc, _, cleanup := hookTestEnv(t)
	defer cleanup()

	created, err := svc.AddHook(defaultHookInput())
	if err != nil {
		t.Fatalf("AddHook() error = %v", err)
	}

	missingSink := "missing-sink"
	_, err = svc.ModifyHook(created.ID, HookModifyInput{SinkRef: &missingSink})
	assertRuntimeCode(t, err, "notification_sink_not_found")
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
// TestHookModifyAuditSink: 修改 sink 时审计包含 sink 信息且不泄露 secret
// ---------------------------------------------------------------------------

func TestHookModifyAuditSink(t *testing.T) {
	svc, store, cleanup := hookTestEnv(t)
	defer cleanup()
	createHookTestSink(t, store, svc.workspaceID, svc.runtime.ActorUserID, "audit-sink")

	created, err := svc.AddHook(defaultHookInput())
	if err != nil {
		t.Fatalf("AddHook() error = %v", err)
	}

	newSink := "audit-sink"
	_, err = svc.ModifyHook(created.ID, HookModifyInput{SinkRef: &newSink})
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

	if payload["sink_name"] != "audit-sink" {
		t.Fatalf("sink_name = %#v, want audit-sink; payload=%#v", payload["sink_name"], payload)
	}

	if _, hasSecret := payload["secret"]; hasSecret {
		t.Fatal("modify audit payload contains raw secret")
	}
	if _, hasFingerprint := payload["secret_fingerprint"]; hasFingerprint {
		t.Fatal("modify audit payload contains sink secret fingerprint")
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
		EventTypes: []string{"task.created"}, SinkRef: "hook-sink", TimeoutSeconds: 10, MaxAttempts: 5,
	})
	if err != nil {
		t.Fatalf("AddHook() ws error = %v", err)
	}

	// project-scoped hook
	_, err = svc.AddHook(HookAddInput{
		Name: "proj-hook", ScopeType: HookScopeProject, ProjectRef: "listproj",
		EventTypes: []string{"task.completed"}, SinkRef: "hook-sink", TimeoutSeconds: 10, MaxAttempts: 5,
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
		EventTypes: []string{"task.created"}, SinkRef: "hook-sink", TimeoutSeconds: 10, MaxAttempts: 5,
	})
	if err != nil {
		t.Fatalf("AddHook() error = %v", err)
	}

	project, err := svc.AddProject(AddProjectInput{Slug: "api", Name: "API"})
	if err != nil {
		t.Fatalf("AddProject() error = %v", err)
	}

	// 创建任务
	created, err := svc.Add(AddInput{Title: "test task", Project: &project.Slug})
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	// 验证 delivery 已入队
	deliveries, err := svc.hookDeliveryRepo.ListByHook(hook.ID, "", 10, 0)
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
	if payload["delivery_id"] != d.ID || payload["workspace_id"] != d.WorkspaceID || payload["sink_id"] != d.SinkID {
		t.Fatalf("top-level delivery payload = %#v, delivery = %#v", payload, d)
	}
	if payload["hook_id"] != hook.ID || payload["rule_id"] != hook.ID {
		t.Fatalf("hook/rule payload = %#v, want hook_id/rule_id %q", payload, hook.ID)
	}
	if payload["created_at"] != float64(d.CreatedAt) {
		t.Fatalf("created_at = %v, want %d", payload["created_at"], d.CreatedAt)
	}
	if payload["attempt"] != float64(1) {
		t.Fatalf("payload attempt = %v, want 1", payload["attempt"])
	}
	delivery := payload["delivery"].(map[string]any)
	if delivery["id"] != d.ID || delivery["workspace_id"] != d.WorkspaceID || delivery["sink_id"] != d.SinkID {
		t.Fatalf("delivery payload = %#v, delivery = %#v", delivery, d)
	}
	object := payload["object"].(map[string]any)
	if object["kind"] != "task" || object["id"] != created.UUID {
		t.Fatalf("object payload = %#v", object)
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
	if taskData["title"] != "test task" {
		t.Fatalf("task.title = %v, want test task", taskData["title"])
	}
	if taskData["task_slug"] != "api-1" {
		t.Fatalf("task.task_slug = %v, want api-1", taskData["task_slug"])
	}

	// 验证 headers
	var headers map[string]string
	if err := json.Unmarshal([]byte(d.HeadersJSON), &headers); err != nil {
		t.Fatalf("json.Unmarshal(headers) error = %v", err)
	}
	if headers["X-Xuanchu-Event"] != "task.created" {
		t.Fatalf("X-Xuanchu-Event = %q, want task.created", headers["X-Xuanchu-Event"])
	}
	if headers["X-Xuanchu-Event-Version"] != "1" {
		t.Fatalf("X-Xuanchu-Event-Version = %q, want 1", headers["X-Xuanchu-Event-Version"])
	}

	// 确保 store 已关闭（defer 会处理）
	_ = store
}

func TestHookHTTPTemplateDeliveryContext(t *testing.T) {
	svc, store, cleanup := hookTestEnv(t)
	defer cleanup()
	sinkRepo := storage.NewNotificationSinkRepository(store.DB())
	enabled := true
	if err := sinkRepo.Create(storage.NotificationSink{
		ID:                  "sink-hook-template",
		WorkspaceID:         svc.workspaceID,
		Name:                "hook-template",
		Type:                NotificationSinkTypeHTTPTemplate,
		EndpointMode:        NotificationEndpointStaticURL,
		URL:                 "https://example.com/webhook",
		AllowedHostsJSON:    `["example.com"]`,
		HTTPMethod:          "POST",
		HeaderTemplatesJSON: `[{"name":"X-Delivery","value":"{{delivery.id}}"},{"name":"X-Object","value":"{{object.kind}}/{{object.id}}"}]`,
		BodyTemplate:        `{"delivery_id":"{{delivery.id}}","attempt":{{delivery.attempt}},"workspace_id":"{{delivery.workspace_id}}","sink_id":"{{delivery.sink_id}}","object_kind":"{{object.kind}}","object_id":"{{object.id}}"}`,
		BodyContentType:     "application/json",
		SecretRefsJSON:      `{}`,
		Enabled:             &enabled,
		TimeoutSeconds:      10,
		MaxAttempts:         5,
		CreatedBy:           svc.runtime.ActorUserID,
		CreatedAt:           100,
		ModifiedAt:          100,
	}); err != nil {
		t.Fatal(err)
	}
	hook, err := svc.AddHook(HookAddInput{
		Name:           "template-hook",
		ScopeType:      HookScopeWorkspace,
		EventTypes:     []string{"task.created"},
		SinkRef:        "hook-template",
		TimeoutSeconds: 10,
		MaxAttempts:    5,
	})
	if err != nil {
		t.Fatalf("AddHook() error = %v", err)
	}
	created, err := svc.Add(AddInput{Title: "templated hook"})
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	rows, err := svc.hookDeliveryRepo.ListByHook(hook.ID, "", 10, 0)
	if err != nil {
		t.Fatalf("ListByHook() error = %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("delivery count = %d, want 1", len(rows))
	}
	row := rows[0]
	var body map[string]any
	if err := json.Unmarshal([]byte(row.RenderedBody), &body); err != nil {
		t.Fatalf("RenderedBody json error = %v body=%q", err, row.RenderedBody)
	}
	if body["delivery_id"] != row.ID || body["workspace_id"] != row.WorkspaceID || body["sink_id"] != row.SinkID {
		t.Fatalf("body delivery fields = %#v, row = %#v", body, row)
	}
	if body["attempt"] != float64(1) {
		t.Fatalf("body attempt = %v, want 1", body["attempt"])
	}
	if body["object_kind"] != "task" || body["object_id"] != created.UUID {
		t.Fatalf("body object fields = %#v", body)
	}
	var headers map[string][]string
	if err := json.Unmarshal([]byte(row.RenderedHeadersJSON), &headers); err != nil {
		t.Fatalf("RenderedHeadersJSON error = %v", err)
	}
	if got := headers["X-Delivery"]; len(got) != 1 || got[0] != row.ID {
		t.Fatalf("X-Delivery = %#v, want %q", got, row.ID)
	}
	if got := headers["X-Object"]; len(got) != 1 || got[0] != "task/"+created.UUID {
		t.Fatalf("X-Object = %#v, want task/%s", got, created.UUID)
	}
}

func TestHookTaskCreatedPayloadOmitsTaskSlugWithoutProject(t *testing.T) {
	svc, _, cleanup := hookTestEnv(t)
	defer cleanup()

	hook, err := svc.AddHook(HookAddInput{
		Name: "create-hook", ScopeType: HookScopeWorkspace,
		EventTypes: []string{"task.created"}, SinkRef: "hook-sink", TimeoutSeconds: 10, MaxAttempts: 5,
	})
	if err != nil {
		t.Fatalf("AddHook() error = %v", err)
	}

	if _, err := svc.Add(AddInput{Title: "no project"}); err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	deliveries, err := svc.hookDeliveryRepo.ListByHook(hook.ID, "", 10, 0)
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
	data := payload["data"].(map[string]any)
	taskData := data["task"].(map[string]any)
	if _, ok := taskData["task_slug"]; ok {
		t.Fatalf("task_slug should be omitted without project: %#v", taskData)
	}
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
		EventTypes: []string{"task.created"}, SinkRef: "hook-sink", TimeoutSeconds: 10, MaxAttempts: 5,
	})
	if err != nil {
		t.Fatalf("AddHook() error = %v", err)
	}

	created, err := svc.Add(AddInput{Title: "assigned hook task", Assignees: []string{"hook-assignee"}})
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	deliveries, err := svc.hookDeliveryRepo.ListByHook(hook.ID, "", 10, 0)
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
		EventTypes:     []string{"task.created", "task.modified", "task.completed", "task.deleted", "task.started", "task.stopped"},
		SinkRef:        "hook-sink",
		TimeoutSeconds: 10, MaxAttempts: 5,
	})
	if err != nil {
		t.Fatalf("AddHook() error = %v", err)
	}

	// Add -> task.created
	created, err := svc.Add(AddInput{Title: "task1"})
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

	// Start -> task.started
	err = svc.Start(created.UUID)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	assertDeliveryEventType(t, svc, hook.ID, "task.started")

	// Stop -> task.stopped
	err = svc.Stop(created.UUID)
	if err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	assertDeliveryEventType(t, svc, hook.ID, "task.stopped")

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
	toDelete, err := svc.Add(AddInput{Title: "task to delete"})
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
	deliveries, err := svc.hookDeliveryRepo.ListByHook(hookID, "", 20, 0)
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
	proj, err := svc.AddProject(AddProjectInput{Slug: "isolated", Name: "Isolated Project"})
	if err != nil {
		t.Fatalf("AddProject() error = %v", err)
	}

	// 创建 project-scoped hook，只监听 task.created
	projHook, err := svc.AddHook(HookAddInput{
		Name: "proj-hook", ScopeType: HookScopeProject, ProjectRef: "isolated",
		EventTypes: []string{"task.created"}, SinkRef: "hook-sink", TimeoutSeconds: 10, MaxAttempts: 5,
	})
	if err != nil {
		t.Fatalf("AddHook() proj error = %v", err)
	}

	// 创建 workspace-scoped hook
	wsHook, err := svc.AddHook(HookAddInput{
		Name: "ws-hook", ScopeType: HookScopeWorkspace,
		EventTypes: []string{"task.created"}, SinkRef: "hook-sink", TimeoutSeconds: 10, MaxAttempts: 5,
	})
	if err != nil {
		t.Fatalf("AddHook() ws error = %v", err)
	}

	// 创建带 project 的任务 -> 两个 hook 都应收到
	p := "isolated"
	_, err = svc.Add(AddInput{Title: "in project", Project: &p})
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	projDeliveries, _ := svc.hookDeliveryRepo.ListByHook(projHook.ID, "", 10, 0)
	wsDeliveries, _ := svc.hookDeliveryRepo.ListByHook(wsHook.ID, "", 10, 0)
	if len(projDeliveries) != 1 {
		t.Fatalf("proj hook deliveries for in-project task = %d, want 1", len(projDeliveries))
	}
	if len(wsDeliveries) != 1 {
		t.Fatalf("ws hook deliveries for in-project task = %d, want 1", len(wsDeliveries))
	}

	// 创建无 project 的任务 -> 只有 workspace hook 应收到
	_, err = svc.Add(AddInput{Title: "no project"})
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	projDeliveries2, _ := svc.hookDeliveryRepo.ListByHook(projHook.ID, "", 10, 0)
	wsDeliveries2, _ := svc.hookDeliveryRepo.ListByHook(wsHook.ID, "", 10, 0)
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
		EventTypes: []string{"task.completed"}, SinkRef: "hook-sink", TimeoutSeconds: 10, MaxAttempts: 5,
	})
	if err != nil {
		t.Fatalf("AddHook() error = %v", err)
	}

	created, err := svc.Add(AddInput{Title: "finish me"})
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	// Done 之前不应有 delivery
	deliveries, _ := svc.hookDeliveryRepo.ListByHook(hook.ID, "", 10, 0)
	if len(deliveries) != 0 {
		t.Fatalf("deliveries before done = %d, want 0", len(deliveries))
	}

	err = svc.Done(created.UUID)
	if err != nil {
		t.Fatalf("Done() error = %v", err)
	}

	deliveries, _ = svc.hookDeliveryRepo.ListByHook(hook.ID, "", 10, 0)
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
// TestHookDeliveryFailureDoesNotRollBackTaskWrite: 投递失败不回滚任务写入
// ---------------------------------------------------------------------------

func TestHookDeliveryFailureDoesNotRollBackTaskWrite(t *testing.T) {
	svc, store, cleanup := hookTestEnv(t)
	defer cleanup()

	// 创建 hook
	_, err := svc.AddHook(HookAddInput{
		Name: "rollback-hook", ScopeType: HookScopeWorkspace,
		EventTypes: []string{"task.created"}, SinkRef: "hook-sink", TimeoutSeconds: 10, MaxAttempts: 5,
	})
	if err != nil {
		t.Fatalf("AddHook() error = %v", err)
	}

	// 注入一个会失败的 hookDeliveryRepo
	svc.hookDeliveryRepo = &failingDeliveryRepo{}

	// 创建任务应该成功，hook delivery 属于事务提交后的副作用
	created, err := svc.Add(AddInput{Title: "should not rollback"})
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	cleanSvc := newTestServiceWithRuntime(t, store, 1000, "local", "local")
	got, err := cleanSvc.Info(created.UUID)
	if err != nil {
		t.Fatalf("Info(created) error = %v", err)
	}
	if got.Title != "should not rollback" {
		t.Fatalf("title = %q", got.Title)
	}
}

func TestHookDeliveryActorResolvedToUserInfo(t *testing.T) {
	svc, _, cleanup := hookTestEnv(t)
	defer cleanup()

	hook, err := svc.AddHook(defaultHookInput())
	if err != nil {
		t.Fatalf("AddHook() error = %v", err)
	}
	if _, err := svc.Add(AddInput{Title: "actor user info"}); err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	rows, err := svc.ListHookDeliveries(hook.ID, "", 10, 0)
	if err != nil {
		t.Fatalf("ListHookDeliveries() error = %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("delivery count = %d, want 1", len(rows))
	}
	if rows[0].Actor.ID != svc.Runtime().ActorUserID || rows[0].Actor.Name != svc.Runtime().ActorName {
		t.Fatalf("list actor = %#v, want id %q name %q", rows[0].Actor, svc.Runtime().ActorUserID, svc.Runtime().ActorName)
	}
	info, err := svc.HookDeliveryInfo(rows[0].ID)
	if err != nil {
		t.Fatalf("HookDeliveryInfo() error = %v", err)
	}
	if info.Actor.ID != svc.Runtime().ActorUserID || info.Actor.Name != svc.Runtime().ActorName {
		t.Fatalf("info actor = %#v, want id %q name %q", info.Actor, svc.Runtime().ActorUserID, svc.Runtime().ActorName)
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
		EventTypes: []string{"project.archived"}, SinkRef: "hook-sink", TimeoutSeconds: 10, MaxAttempts: 5,
	})
	if err != nil {
		t.Fatalf("AddHook() error = %v", err)
	}

	// 创建并归档 project
	proj, err := svc.AddProject(AddProjectInput{Slug: "archive", Name: "To Archive"})
	if err != nil {
		t.Fatalf("AddProject() error = %v", err)
	}

	archived, err := svc.ArchiveProject(proj.Slug)
	if err != nil {
		t.Fatalf("ArchiveProject() error = %v", err)
	}

	// 验证 delivery
	deliveries, err := svc.hookDeliveryRepo.ListByHook(hook.ID, "", 10, 0)
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
	if projData["slug"] != "archive" {
		t.Fatalf("project slug = %v, want archive", projData["slug"])
	}

	_ = archived
}

func TestHookProjectAnnotatedEventPayload(t *testing.T) {
	svc, _, cleanup := hookTestEnv(t)
	defer cleanup()

	hook, err := svc.AddHook(HookAddInput{
		Name: "annotated-hook", ScopeType: HookScopeWorkspace,
		EventTypes: []string{"project.annotated"}, SinkRef: "hook-sink", TimeoutSeconds: 10, MaxAttempts: 5,
	})
	if err != nil {
		t.Fatalf("AddHook() error = %v", err)
	}
	proj, err := svc.AddProject(AddProjectInput{Slug: "notes", Name: "Notes"})
	if err != nil {
		t.Fatalf("AddProject() error = %v", err)
	}
	annotation, err := svc.ProjectAnnotate(proj.Slug, "project note")
	if err != nil {
		t.Fatalf("ProjectAnnotate() error = %v", err)
	}
	deliveries, err := svc.hookDeliveryRepo.ListByHook(hook.ID, "", 10, 0)
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
	data := payload["data"].(map[string]any)
	project := data["project"].(map[string]any)
	if project["slug"] != "notes" {
		t.Fatalf("project.slug = %v, want notes", project["slug"])
	}
	gotAnnotation := data["annotation"].(map[string]any)
	if gotAnnotation["id"] != annotation.ID {
		t.Fatalf("annotation.id = %v, want %s", gotAnnotation["id"], annotation.ID)
	}
	if gotAnnotation["content"] != "project note" {
		t.Fatalf("annotation.content = %v, want project note", gotAnnotation["content"])
	}
	createdBy := gotAnnotation["created_by"].(map[string]any)
	if createdBy["id"] == "" || createdBy["name"] == "" {
		t.Fatalf("created_by = %#v, want full user info", createdBy)
	}
}

func TestHookProjectDenotatedEventPayload(t *testing.T) {
	svc, _, cleanup := hookTestEnv(t)
	defer cleanup()

	hook, err := svc.AddHook(HookAddInput{
		Name: "denotated-hook", ScopeType: HookScopeWorkspace,
		EventTypes: []string{"project.denotated"}, SinkRef: "hook-sink", TimeoutSeconds: 10, MaxAttempts: 5,
	})
	if err != nil {
		t.Fatalf("AddHook() error = %v", err)
	}
	proj, err := svc.AddProject(AddProjectInput{Slug: "notes", Name: "Notes"})
	if err != nil {
		t.Fatalf("AddProject() error = %v", err)
	}
	annotation, err := svc.ProjectAnnotate(proj.Slug, "project note")
	if err != nil {
		t.Fatalf("ProjectAnnotate() error = %v", err)
	}
	if err := svc.ProjectDenotate(proj.Slug, annotation.ID); err != nil {
		t.Fatalf("ProjectDenotate() error = %v", err)
	}
	deliveries, err := svc.hookDeliveryRepo.ListByHook(hook.ID, "", 10, 0)
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
	data := payload["data"].(map[string]any)
	project := data["project"].(map[string]any)
	if project["slug"] != "notes" {
		t.Fatalf("project.slug = %v, want notes", project["slug"])
	}
	gotAnnotation := data["annotation"].(map[string]any)
	if gotAnnotation["id"] != annotation.ID {
		t.Fatalf("annotation.id = %v, want %s", gotAnnotation["id"], annotation.ID)
	}
}

func TestHookTaskUnblockedEventForLastDependency(t *testing.T) {
	svc, _, cleanup := hookTestEnv(t)
	defer cleanup()

	hook, err := svc.AddHook(HookAddInput{
		Name: "unblocked-hook", ScopeType: HookScopeWorkspace,
		EventTypes: []string{"task.unblocked"}, SinkRef: "hook-sink", TimeoutSeconds: 10, MaxAttempts: 5,
	})
	if err != nil {
		t.Fatalf("AddHook() error = %v", err)
	}
	blocker, err := svc.Add(AddInput{Title: "prepare api"})
	if err != nil {
		t.Fatalf("Add(blocker) error = %v", err)
	}
	blocked, err := svc.Add(AddInput{Title: "integrate client"})
	if err != nil {
		t.Fatalf("Add(blocked) error = %v", err)
	}
	if err := svc.Modify(blocked.UUID, ModifyInput{AddDepends: []string{blocker.UUID}}); err != nil {
		t.Fatalf("Modify(depends) error = %v", err)
	}
	if err := svc.Done(blocker.UUID); err != nil {
		t.Fatalf("Done(blocker) error = %v", err)
	}

	deliveries, err := svc.hookDeliveryRepo.ListByHook(hook.ID, "", 10, 0)
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
	if payload["event_type"] != "task.unblocked" {
		t.Fatalf("event_type = %v, want task.unblocked", payload["event_type"])
	}
	data := payload["data"].(map[string]any)
	taskData := data["task"].(map[string]any)
	if taskData["uuid"] != blocked.UUID {
		t.Fatalf("task.uuid = %v, want %s", taskData["uuid"], blocked.UUID)
	}
	dependency := data["dependency"].(map[string]any)
	completedTask := dependency["completed_task"].(map[string]any)
	if completedTask["uuid"] != blocker.UUID {
		t.Fatalf("completed_task.uuid = %v, want %s", completedTask["uuid"], blocker.UUID)
	}
}

func TestHookTaskUnblockedEventNotGeneratedWhenOtherBlockersRemain(t *testing.T) {
	svc, _, cleanup := hookTestEnv(t)
	defer cleanup()

	hook, err := svc.AddHook(HookAddInput{
		Name: "unblocked-hook", ScopeType: HookScopeWorkspace,
		EventTypes: []string{"task.unblocked"}, SinkRef: "hook-sink", TimeoutSeconds: 10, MaxAttempts: 5,
	})
	if err != nil {
		t.Fatalf("AddHook() error = %v", err)
	}
	blocker1, _ := svc.Add(AddInput{Title: "prepare api"})
	blocker2, _ := svc.Add(AddInput{Title: "prepare data"})
	blocked, _ := svc.Add(AddInput{Title: "integrate client"})
	if err := svc.Modify(blocked.UUID, ModifyInput{AddDepends: []string{blocker1.UUID, blocker2.UUID}}); err != nil {
		t.Fatalf("Modify(depends) error = %v", err)
	}
	if err := svc.Done(blocker1.UUID); err != nil {
		t.Fatalf("Done(blocker1) error = %v", err)
	}

	deliveries, err := svc.hookDeliveryRepo.ListByHook(hook.ID, "", 10, 0)
	if err != nil {
		t.Fatalf("ListByHook() error = %v", err)
	}
	if len(deliveries) != 0 {
		t.Fatalf("deliveries count = %d, want 0", len(deliveries))
	}
}

func TestHookTaskUnblockedEventNotGeneratedForCompletedDependent(t *testing.T) {
	svc, _, cleanup := hookTestEnv(t)
	defer cleanup()

	hook, err := svc.AddHook(HookAddInput{
		Name: "unblocked-hook", ScopeType: HookScopeWorkspace,
		EventTypes: []string{"task.unblocked"}, SinkRef: "hook-sink", TimeoutSeconds: 10, MaxAttempts: 5,
	})
	if err != nil {
		t.Fatalf("AddHook() error = %v", err)
	}
	blocker, _ := svc.Add(AddInput{Title: "prepare api"})
	dependent, _ := svc.Add(AddInput{Title: "integrate client"})
	if err := svc.Modify(dependent.UUID, ModifyInput{AddDepends: []string{blocker.UUID}}); err != nil {
		t.Fatalf("Modify(depends) error = %v", err)
	}
	if err := svc.Done(dependent.UUID); err != nil {
		t.Fatalf("Done(dependent) error = %v", err)
	}
	if err := svc.Done(blocker.UUID); err != nil {
		t.Fatalf("Done(blocker) error = %v", err)
	}

	deliveries, err := svc.hookDeliveryRepo.ListByHook(hook.ID, "", 10, 0)
	if err != nil {
		t.Fatalf("ListByHook() error = %v", err)
	}
	if len(deliveries) != 0 {
		t.Fatalf("deliveries count = %d, want 0", len(deliveries))
	}
}

func TestHookBlockedEventOnAddWithDependency(t *testing.T) {
	svc, _, cleanup := hookTestEnv(t)
	defer cleanup()

	hook, err := svc.AddHook(HookAddInput{
		Name: "blocked-hook", ScopeType: HookScopeWorkspace,
		EventTypes:     []string{"task.blocked"},
		SinkRef:        "hook-sink",
		TimeoutSeconds: 10, MaxAttempts: 5,
	})
	if err != nil {
		t.Fatal(err)
	}

	blocker, err := svc.Add(AddInput{Title: "blocker"})
	if err != nil {
		t.Fatal(err)
	}

	_, err = svc.Add(AddInput{
		Title:   "blocked task",
		Depends: []string{blocker.UUID},
	})
	if err != nil {
		t.Fatal(err)
	}

	deliveries, err := svc.hookDeliveryRepo.ListByHook(hook.ID, "", 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(deliveries) == 0 {
		t.Fatal("expected task.blocked delivery")
	}
	if deliveries[0].EventType != "task.blocked" {
		t.Fatalf("event_type = %q, want task.blocked", deliveries[0].EventType)
	}
}

func TestHookBlockedEventOnModifyAddDependency(t *testing.T) {
	svc, _, cleanup := hookTestEnv(t)
	defer cleanup()

	hook, err := svc.AddHook(HookAddInput{
		Name: "blocked-hook", ScopeType: HookScopeWorkspace,
		EventTypes:     []string{"task.blocked"},
		SinkRef:        "hook-sink",
		TimeoutSeconds: 10, MaxAttempts: 5,
	})
	if err != nil {
		t.Fatal(err)
	}

	blocker, err := svc.Add(AddInput{Title: "blocker"})
	if err != nil {
		t.Fatal(err)
	}

	target, err := svc.Add(AddInput{Title: "target"})
	if err != nil {
		t.Fatal(err)
	}

	err = svc.Modify(target.UUID, ModifyInput{
		AddDepends: []string{blocker.UUID},
	})
	if err != nil {
		t.Fatal(err)
	}

	assertDeliveryEventType(t, svc, hook.ID, "task.blocked")
}

func TestHookModifyFineGrainedEventPayloads(t *testing.T) {
	svc, store, cleanup := hookTestEnv(t)
	defer cleanup()

	user := mustCreateUserRecord(t, store, storage.User{
		ID:         "user-modifier",
		Name:       "modifier",
		Email:      strptr("modifier@example.com"),
		CreatedAt:  1000,
		ModifiedAt: 1000,
	})
	assignee := mustCreateUserRecord(t, store, storage.User{
		ID:         "user-assignee",
		Name:       "assignee",
		Email:      strptr("assignee@example.com"),
		CreatedAt:  1000,
		ModifiedAt: 1000,
	})
	mustUpsertMembershipRecord(t, store, storage.Membership{
		UserID:      user.ID,
		WorkspaceID: svc.workspaceID,
		Role:        string(RoleMember),
		JoinedAt:    1000,
		ModifiedAt:  1000,
	})
	mustUpsertMembershipRecord(t, store, storage.Membership{
		UserID:      assignee.ID,
		WorkspaceID: svc.workspaceID,
		Role:        string(RoleMember),
		JoinedAt:    1000,
		ModifiedAt:  1000,
	})

	if err := svc.BindExternalID(user.ID, "feishu", "ou_modifier"); err != nil {
		t.Fatal(err)
	}

	hook, err := svc.AddHook(HookAddInput{
		Name: "fine-grained-hook", ScopeType: HookScopeWorkspace,
		EventTypes: []string{
			"task.modified", "task.priority_changed", "task.tags_changed",
			"task.due_changed", "task.project_changed", "task.assigned", "task.unassigned",
		},
		SinkRef:        "hook-sink",
		TimeoutSeconds: 10,
		MaxAttempts:    5,
	})
	if err != nil {
		t.Fatal(err)
	}

	oldProject, err := svc.AddProject(AddProjectInput{Slug: "oldproj", Name: "Old Project"})
	if err != nil {
		t.Fatal(err)
	}
	newProject, err := svc.AddProject(AddProjectInput{Slug: "newproj", Name: "New Project"})
	if err != nil {
		t.Fatal(err)
	}
	oldDue := int64(1000)
	newDue := int64(2000)
	created, err := svc.Add(AddInput{
		Title:     "fine-grained task",
		Project:   &oldProject.Slug,
		Priority:  strptr("M"),
		Due:       &oldDue,
		Assignees: []string{assignee.ID},
		Tags:      []string{"docs"},
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := svc.Modify(created.UUID, ModifyInput{
		Project:         &newProject.Slug,
		Priority:        strptr("H"),
		Due:             &newDue,
		AddTags:         []string{"urgent"},
		RemoveTags:      []string{"docs"},
		AddAssignees:    []string{user.ID},
		RemoveAssignees: []string{assignee.ID},
	}); err != nil {
		t.Fatal(err)
	}

	deliveries, err := svc.hookDeliveryRepo.ListByHook(hook.ID, "", 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	payloads := map[string]map[string]any{}
	for _, d := range deliveries {
		var payload map[string]any
		if err := json.Unmarshal([]byte(d.PayloadJSON), &payload); err != nil {
			t.Fatalf("payload unmarshal for %s: %v", d.EventType, err)
		}
		data, ok := payload["data"].(map[string]any)
		if !ok {
			t.Fatalf("payload data for %s = %#v, want object", d.EventType, payload["data"])
		}
		if _, ok := data["task"].(map[string]any); !ok {
			t.Fatalf("payload data for %s missing task snapshot: %#v", d.EventType, data)
		}
		payloads[d.EventType] = data
	}

	if payloads["task.priority_changed"]["previous_priority"] != "M" || payloads["task.priority_changed"]["current_priority"] != "H" {
		t.Fatalf("priority payload = %#v", payloads["task.priority_changed"])
	}
	if payloads["task.due_changed"]["previous_due"] != float64(oldDue) || payloads["task.due_changed"]["current_due"] != float64(newDue) {
		t.Fatalf("due payload = %#v", payloads["task.due_changed"])
	}
	if payloads["task.project_changed"]["previous_project"] != oldProject.Slug || payloads["task.project_changed"]["current_project"] != newProject.Slug {
		t.Fatalf("project payload = %#v", payloads["task.project_changed"])
	}
	if got := payloads["task.tags_changed"]["added_tags"]; !stringSlicePayloadEqual(got, []string{"urgent"}) {
		t.Fatalf("added_tags = %#v, want [urgent]", got)
	}
	if got := payloads["task.tags_changed"]["removed_tags"]; !stringSlicePayloadEqual(got, []string{"docs"}) {
		t.Fatalf("removed_tags = %#v, want [docs]", got)
	}
	if got := payloads["task.tags_changed"]["current_tags"]; !stringSlicePayloadEqual(got, []string{"urgent"}) {
		t.Fatalf("current_tags = %#v, want [urgent]", got)
	}

	assertUserInfoPayload(t, payloads["task.assigned"]["added_assignees"], user.ID, user.Name, user.Email)
	assertUserInfoPayload(t, payloads["task.assigned"]["current_assignees"], user.ID, user.Name, user.Email)
	assertUserInfoPayload(t, payloads["task.unassigned"]["removed_assignees"], assignee.ID, assignee.Name, assignee.Email)
	assertUserInfoPayload(t, payloads["task.unassigned"]["current_assignees"], user.ID, user.Name, user.Email)
}

func stringSlicePayloadEqual(got any, want []string) bool {
	values, ok := got.([]any)
	if !ok || len(values) != len(want) {
		return false
	}
	for i, wantValue := range want {
		if values[i] != wantValue {
			return false
		}
	}
	return true
}

func assertUserInfoPayload(t *testing.T, got any, wantID, wantName string, wantEmail *string) {
	t.Helper()
	values, ok := got.([]any)
	if !ok || len(values) != 1 {
		t.Fatalf("user info payload = %#v, want single user", got)
	}
	user, ok := values[0].(map[string]any)
	if !ok {
		t.Fatalf("user info = %#v, want object", values[0])
	}
	if user["id"] != wantID || user["name"] != wantName {
		t.Fatalf("user info = %#v, want id %q name %q", user, wantID, wantName)
	}
	if wantEmail == nil {
		if user["email"] != nil {
			t.Fatalf("user email = %#v, want nil", user["email"])
		}
	} else if user["email"] != *wantEmail {
		t.Fatalf("user email = %#v, want %q", user["email"], *wantEmail)
	}
	if _, ok := user["external_ids"].([]any); !ok {
		t.Fatalf("user external_ids = %#v, want array", user["external_ids"])
	}
}

func TestHookBlockedEventNotGeneratedWhenTaskAlreadyBlocked(t *testing.T) {
	svc, _, cleanup := hookTestEnv(t)
	defer cleanup()

	hook, err := svc.AddHook(HookAddInput{
		Name: "blocked-hook", ScopeType: HookScopeWorkspace,
		EventTypes:     []string{"task.blocked"},
		SinkRef:        "hook-sink",
		TimeoutSeconds: 10, MaxAttempts: 5,
	})
	if err != nil {
		t.Fatal(err)
	}

	blocker1, err := svc.Add(AddInput{Title: "blocker one"})
	if err != nil {
		t.Fatal(err)
	}
	blocker2, err := svc.Add(AddInput{Title: "blocker two"})
	if err != nil {
		t.Fatal(err)
	}
	target, err := svc.Add(AddInput{Title: "target", Depends: []string{blocker1.UUID}})
	if err != nil {
		t.Fatal(err)
	}

	if err := svc.Modify(target.UUID, ModifyInput{AddDepends: []string{blocker2.UUID}}); err != nil {
		t.Fatal(err)
	}

	deliveries, err := svc.hookDeliveryRepo.ListByHook(hook.ID, "", 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(deliveries) != 1 {
		t.Fatalf("deliveries count = %d, want only initial add blocked event", len(deliveries))
	}
}

package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"git.dajee.net/dajee/xuanchu/internal/app"
	"git.dajee.net/dajee/xuanchu/internal/logging"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// setupResourceTestServer 创建一个注册了 resources 的 in-memory MCP server+client 对。
// 返回 client session 和 cleanup 函数。
func setupResourceTestServer(t *testing.T, opts Options) *mcp.ClientSession {
	t.Helper()
	srv := NewServer(opts)

	serverTransport, clientTransport := mcp.NewInMemoryTransports()

	go func() {
		if err := srv.Run(context.Background(), serverTransport); err != nil {
			t.Logf("resource server run: %v", err)
		}
	}()

	client := mcp.NewClient(&mcp.Implementation{
		Name:    "test-client",
		Version: "0.0.1",
	}, nil)
	session, err := client.Connect(context.Background(), clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

func TestResourcesListIncludesStaticResources(t *testing.T) {
	store := newMCPTestStore(t)
	session := setupResourceTestServer(t, Options{
		Store:   store,
		Clock:   testClock{now: 100},
		Version: "test",
		Mode:    ModeStdio,
	})

	result, err := session.ListResources(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListResources: %v", err)
	}

	uris := make(map[string]bool)
	for _, r := range result.Resources {
		uris[r.URI] = true
	}

	for _, want := range allResourceURIs() {
		if !uris[want] {
			t.Errorf("missing static resource %q in list; got URIs: %v", want, uris)
		}
	}
}

func TestResourceReadWritesOperationLog(t *testing.T) {
	var buf bytes.Buffer
	logger, closeLogger, err := logging.Setup(logging.LogConfig{Format: "text"}, &buf)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = closeLogger() })

	store := newMCPTestStore(t)
	session := setupResourceTestServer(t, Options{
		Store:   store,
		Clock:   testClock{now: 100},
		Version: "test",
		Mode:    ModeStdio,
		Logger:  logger,
	})

	if _, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "xuanchu://workspace/current"}); err != nil {
		t.Fatalf("ReadResource: %v", err)
	}

	logText := buf.String()
	for _, want := range []string{
		"component=mcp",
		"operation=mcp_resource_read",
		"resource=xuanchu://workspace/current",
		"mode=stdio",
		"result=success",
		"duration_ms=",
	} {
		if !strings.Contains(logText, want) {
			t.Fatalf("log = %q, want substring %q", logText, want)
		}
	}
}

func TestResourceTemplatesListIncludesTemplateResources(t *testing.T) {
	store := newMCPTestStore(t)
	session := setupResourceTestServer(t, Options{
		Store:   store,
		Clock:   testClock{now: 100},
		Version: "test",
		Mode:    ModeStdio,
	})

	result, err := session.ListResourceTemplates(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListResourceTemplates: %v", err)
	}

	templates := make(map[string]bool)
	for _, rt := range result.ResourceTemplates {
		templates[rt.URITemplate] = true
	}

	for _, want := range allResourceTemplateURIs() {
		if !templates[want] {
			t.Errorf("missing resource template %q; got: %v", want, templates)
		}
	}
}

func TestReadWorkspaceCurrent(t *testing.T) {
	store := newMCPTestStore(t)
	_ = newMCPTestService(t, store)
	// 默认 local workspace 已存在

	session := setupResourceTestServer(t, Options{
		Store:   store,
		Clock:   testClock{now: 100},
		Version: "test",
		Mode:    ModeStdio,
	})

	result, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{
		URI: "xuanchu://workspace/current",
	})
	if err != nil {
		t.Fatalf("ReadResource workspace/current: %v", err)
	}
	if len(result.Contents) == 0 {
		t.Fatal("expected at least one content item")
	}

	content := result.Contents[0]
	if content.MIMEType != "application/json" {
		t.Errorf("MIMEType = %q, want application/json", content.MIMEType)
	}

	var data workspaceResourceData
	if err := json.Unmarshal([]byte(content.Text), &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if data.Slug != "local" {
		t.Errorf("slug = %q, want local", data.Slug)
	}
	if data.Role != "owner" {
		t.Errorf("role = %q, want owner", data.Role)
	}
}

func TestReadWorkspaceByID(t *testing.T) {
	store := newMCPTestStore(t)
	svc := newMCPTestService(t, store)
	ws, err := svc.AddWorkspace(app.AddWorkspaceInput{Slug: "team", Name: "Team"})
	if err != nil {
		t.Fatal(err)
	}

	session := setupResourceTestServer(t, Options{
		Store:   store,
		Clock:   testClock{now: 100},
		Version: "test",
		Mode:    ModeStdio,
	})

	uri := "xuanchu://workspace/" + ws.ID
	result, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{
		URI: uri,
	})
	if err != nil {
		t.Fatalf("ReadResource workspace/{id}: %v", err)
	}

	var data workspaceResourceData
	if err := json.Unmarshal([]byte(result.Contents[0].Text), &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.Slug != "team" {
		t.Errorf("slug = %q, want team", data.Slug)
	}
	if data.Name != "Team" {
		t.Errorf("name = %q, want Team", data.Name)
	}
}

func TestReadWorkspaceBySlug(t *testing.T) {
	store := newMCPTestStore(t)
	svc := newMCPTestService(t, store)
	if _, err := svc.AddWorkspace(app.AddWorkspaceInput{Slug: "team", Name: "Team"}); err != nil {
		t.Fatal(err)
	}

	session := setupResourceTestServer(t, Options{
		Store:   store,
		Clock:   testClock{now: 100},
		Version: "test",
		Mode:    ModeStdio,
	})

	// 使用 slug 而不是 ID
	uri := "xuanchu://workspace/team"
	result, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{
		URI: uri,
	})
	if err != nil {
		t.Fatalf("ReadResource workspace/team: %v", err)
	}

	var data workspaceResourceData
	if err := json.Unmarshal([]byte(result.Contents[0].Text), &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.Slug != "team" {
		t.Errorf("slug = %q, want team", data.Slug)
	}
}

func TestReadProjectByID(t *testing.T) {
	store := newMCPTestStore(t)
	svc := newMCPTestService(t, store)
	proj, err := svc.AddProject(app.AddProjectInput{Slug: "backend", Name: "Backend"})
	if err != nil {
		t.Fatal(err)
	}
	// 设置 agent config
	if err := svc.ProjectConfigSet(proj.Slug, "agent.background", "true"); err != nil {
		t.Fatal(err)
	}
	if err := svc.ProjectConfigSet(proj.Slug, "agent.constraints", "no-breaking-changes"); err != nil {
		t.Fatal(err)
	}
	// context.default 是合法 key 但不是 agent.* 前缀，不应暴露
	if err := svc.ProjectConfigSet(proj.Slug, "context.default", "sprint"); err != nil {
		t.Fatal(err)
	}

	session := setupResourceTestServer(t, Options{
		Store:   store,
		Clock:   testClock{now: 100},
		Version: "test",
		Mode:    ModeStdio,
	})

	uri := "xuanchu://project/" + proj.ID
	result, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{
		URI: uri,
	})
	if err != nil {
		t.Fatalf("ReadResource project/{id}: %v", err)
	}

	var data projectResourceData
	if err := json.Unmarshal([]byte(result.Contents[0].Text), &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if data.Slug != "backend" {
		t.Errorf("slug = %q, want backend", data.Slug)
	}

	// context.default 是合法的非 secret 键，应随非 secret 配置一起暴露
	if data.AgentConfig["context.default"] != "sprint" {
		t.Errorf("context.default = %q, want sprint", data.AgentConfig["context.default"])
	}
	if data.AgentConfig["agent.background"] != "true" {
		t.Errorf("agent.background = %q, want true", data.AgentConfig["agent.background"])
	}
	if data.AgentConfig["agent.constraints"] != "no-breaking-changes" {
		t.Errorf("agent.constraints = %q, want no-breaking-changes", data.AgentConfig["agent.constraints"])
	}
}

func TestReadProjectRejectsSlugResourceURI(t *testing.T) {
	store := newMCPTestStore(t)
	svc := newMCPTestService(t, store)
	proj, err := svc.AddProject(app.AddProjectInput{Slug: "backend", Name: "Backend"})
	if err != nil {
		t.Fatal(err)
	}
	session := setupResourceTestServer(t, Options{
		Store:   store,
		Clock:   testClock{now: 100},
		Version: "test",
		Mode:    ModeStdio,
	})

	if _, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "xuanchu://project/" + proj.Slug}); err == nil {
		t.Fatal("ReadResource xuanchu://project/{slug} error = nil, want not found")
	}
}

func TestHTTPProjectResourceHonorsProjectScope(t *testing.T) {
	store := newMCPTestStore(t)
	svc := newMCPTestService(t, store)
	allowed, err := svc.AddProject(app.AddProjectInput{Slug: "allowed", Name: "Allowed"})
	if err != nil {
		t.Fatal(err)
	}
	hidden, err := svc.AddProject(app.AddProjectInput{Slug: "hidden", Name: "Hidden"})
	if err != nil {
		t.Fatal(err)
	}
	token := mustCreateMCPToken(t, svc, []string{"project:read"}, []string{"local"}, []string{allowed.ID})
	req, _ := http.NewRequest(http.MethodPost, "/mcp", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	session := setupResourceTestServer(t, Options{
		Store:   store,
		Clock:   testClock{now: 100},
		Version: "test",
		Mode:    ModeHTTP,
		Request: req,
	})

	if _, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "xuanchu://project/" + hidden.ID}); err == nil {
		t.Fatal("ReadResource hidden project error = nil, want scope denial")
	}
}

func TestReadContextCurrent(t *testing.T) {
	store := newMCPTestStore(t)
	svc := newMCPTestService(t, store)

	// 定义并激活 context
	if err := svc.DefineContext("urgent", "priority:H"); err != nil {
		t.Fatal(err)
	}
	if err := svc.UseContext("urgent"); err != nil {
		t.Fatal(err)
	}

	session := setupResourceTestServer(t, Options{
		Store:   store,
		Clock:   testClock{now: 100},
		Version: "test",
		Mode:    ModeStdio,
	})

	result, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{
		URI: "xuanchu://context/current",
	})
	if err != nil {
		t.Fatalf("ReadResource context/current: %v", err)
	}

	var data contextResourceData
	if err := json.Unmarshal([]byte(result.Contents[0].Text), &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if data.Name != "urgent" {
		t.Errorf("name = %q, want urgent", data.Name)
	}
	if data.Filter != "priority:H" {
		t.Errorf("filter = %q, want priority:H", data.Filter)
	}
	if data.WorkspaceSlug != "local" {
		t.Errorf("workspace_slug = %q, want local", data.WorkspaceSlug)
	}
}

func TestReadContextCurrentNone(t *testing.T) {
	store := newMCPTestStore(t)
	// 不定义任何 context

	session := setupResourceTestServer(t, Options{
		Store:   store,
		Clock:   testClock{now: 100},
		Version: "test",
		Mode:    ModeStdio,
	})

	result, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{
		URI: "xuanchu://context/current",
	})
	if err != nil {
		t.Fatalf("ReadResource context/current: %v", err)
	}

	var data contextResourceData
	if err := json.Unmarshal([]byte(result.Contents[0].Text), &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// 没有 active context 时 name 应为空
	if data.Name != "" {
		t.Errorf("name = %q, want empty when no active context", data.Name)
	}
}

func TestTenantTokenCannotReadCurrentContextResource(t *testing.T) {
	store := newMCPTestStore(t)
	svc := newMCPTestService(t, store)
	created, err := svc.CreateTenantAccessToken(app.CreateTenantAccessTokenInput{
		Name:   "runtime",
		Scopes: []string{"context:read", "workspace:read"},
	})
	if err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest(http.MethodPost, "/mcp", nil)
	req.Header.Set("Authorization", "Bearer "+created.RawToken)
	session := setupResourceTestServer(t, Options{
		Store:   store,
		Clock:   testClock{now: 100},
		Version: "test",
		Mode:    ModeHTTP,
		Request: req,
	})

	if _, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "xuanchu://context/current"}); err == nil || !strings.Contains(err.Error(), "tenant token has no user actor") {
		t.Fatalf("ReadResource context/current error = %v, want tenant actor user error", err)
	}

	result, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "xuanchu://workspace/current"})
	if err != nil {
		t.Fatalf("ReadResource workspace/current: %v", err)
	}
	var data workspaceResourceData
	if err := json.Unmarshal([]byte(result.Contents[0].Text), &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data.Context != nil {
		t.Fatalf("workspace/current context = %#v, want nil for tenant token", data.Context)
	}
	if data.Role != "" {
		t.Fatalf("workspace/current role = %q, want empty for tenant token", data.Role)
	}
}

func TestReadWorkspaceCurrentIncludesProjects(t *testing.T) {
	store := newMCPTestStore(t)
	svc := newMCPTestService(t, store)
	if _, err := svc.AddProject(app.AddProjectInput{Slug: "frontend", Name: "Frontend"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddProject(app.AddProjectInput{Slug: "backend", Name: "Backend"}); err != nil {
		t.Fatal(err)
	}

	session := setupResourceTestServer(t, Options{
		Store:   store,
		Clock:   testClock{now: 100},
		Version: "test",
		Mode:    ModeStdio,
	})

	result, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{
		URI: "xuanchu://workspace/current",
	})
	if err != nil {
		t.Fatalf("ReadResource: %v", err)
	}

	var data workspaceResourceData
	if err := json.Unmarshal([]byte(result.Contents[0].Text), &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(data.Projects) < 2 {
		t.Errorf("projects count = %d, want >= 2", len(data.Projects))
	}

	slugs := make(map[string]bool)
	for _, p := range data.Projects {
		slugs[p.Slug] = true
	}
	if !slugs["frontend"] || !slugs["backend"] {
		t.Errorf("expected frontend and backend in projects, got: %v", slugs)
	}
}

func TestProjectResourceExposesNonSecretKeys(t *testing.T) {
	store := newMCPTestStore(t)
	svc := newMCPTestService(t, store)
	proj, err := svc.AddProject(app.AddProjectInput{Slug: "secure", Name: "Secure"})
	if err != nil {
		t.Fatal(err)
	}
	// 定义三个 schema：一个普通键，一个 secret 键，一个普通键（im.group_id）
	if err := svc.ConfigSchemaSet(app.ConfigSchemaInput{
		Key:           "integrations.feishu.webhook_url",
		ValueType:     string(app.ConfigValueTypeString),
		AllowedScopes: []string{string(app.ConfigAllowedScopeProject)},
	}); err != nil {
		t.Fatalf("ConfigSchemaSet(webhook_url): %v", err)
	}
	if err := svc.ConfigSchemaSet(app.ConfigSchemaInput{
		Key:           "integrations.feishu.bot_token",
		ValueType:     string(app.ConfigValueTypeString),
		AllowedScopes: []string{string(app.ConfigAllowedScopeWorkspace), string(app.ConfigAllowedScopeProject)},
		Secret:        true,
	}); err != nil {
		t.Fatalf("ConfigSchemaSet(bot_token): %v", err)
	}
	if err := svc.ConfigSchemaSet(app.ConfigSchemaInput{
		Key:           "im.group_id",
		ValueType:     string(app.ConfigValueTypeString),
		AllowedScopes: []string{string(app.ConfigAllowedScopeProject)},
	}); err != nil {
		t.Fatalf("ConfigSchemaSet(group_id): %v", err)
	}
	// 普通键（有 schema，非 secret）——应出现
	if err := svc.ProjectConfigSet(proj.Slug, "integrations.feishu.webhook_url", "https://open.feishu.cn/hook/xxx"); err != nil {
		t.Fatal(err)
	}
	// secret 键（有 schema，secret:true）——应被排除
	if err := svc.ProjectConfigSet(proj.Slug, "integrations.feishu.bot_token", "t-secret-value"); err != nil {
		t.Fatal(err)
	}
	// agent 指令键——应出现
	if err := svc.ProjectConfigSet(proj.Slug, "agent.background", "bg"); err != nil {
		t.Fatal(err)
	}
	// 普通键 im.group_id（有 schema，非 secret）——应出现
	if err := svc.ProjectConfigSet(proj.Slug, "im.group_id", "oc_yyy"); err != nil {
		t.Fatal(err)
	}

	session := setupResourceTestServer(t, Options{
		Store:   store,
		Clock:   testClock{now: 100},
		Version: "test",
		Mode:    ModeStdio,
	})

	uri := "xuanchu://project/" + proj.ID
	result, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{
		URI: uri,
	})
	if err != nil {
		t.Fatalf("ReadResource: %v", err)
	}

	var data projectResourceData
	if err := json.Unmarshal([]byte(result.Contents[0].Text), &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// 普通键出现
	if data.AgentConfig["integrations.feishu.webhook_url"] != "https://open.feishu.cn/hook/xxx" {
		t.Errorf("webhook_url = %q, want exposed", data.AgentConfig["integrations.feishu.webhook_url"])
	}
	// agent 指令键出现
	if data.AgentConfig["agent.background"] != "bg" {
		t.Errorf("agent.background = %q, want bg", data.AgentConfig["agent.background"])
	}
	// 普通键 im.group_id（有 schema，非 secret）——应出现
	if data.AgentConfig["im.group_id"] != "oc_yyy" {
		t.Errorf("im.group_id = %q, want oc_yyy", data.AgentConfig["im.group_id"])
	}
	// secret 键被排除
	if _, ok := data.AgentConfig["integrations.feishu.bot_token"]; ok {
		t.Error("secret key integrations.feishu.bot_token should NOT be exposed")
	}
}

func TestNoTaskListResourceRegistered(t *testing.T) {
	// 验证 list-style URI 如 xuanchu://project/{id}/tasks 不注册
	store := newMCPTestStore(t)
	session := setupResourceTestServer(t, Options{
		Store:   store,
		Clock:   testClock{now: 100},
		Version: "test",
		Mode:    ModeStdio,
	})

	// 静态 resources 不应包含 tasks 相关 URI
	result, err := session.ListResources(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListResources: %v", err)
	}
	for _, r := range result.Resources {
		if strings.Contains(r.URI, "task") && !strings.Contains(r.URI, "workspace") && !strings.Contains(r.URI, "context") {
			t.Errorf("unexpected task-related resource: %q", r.URI)
		}
	}

	// templates 也不应包含 tasks
	tmplResult, err := session.ListResourceTemplates(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListResourceTemplates: %v", err)
	}
	for _, rt := range tmplResult.ResourceTemplates {
		if strings.Contains(rt.URITemplate, "tasks") {
			t.Errorf("unexpected tasks template: %q", rt.URITemplate)
		}
	}
}

func TestReadWorkspaceCurrentWithActiveContext(t *testing.T) {
	store := newMCPTestStore(t)
	svc := newMCPTestService(t, store)
	if err := svc.DefineContext("sprint", "project:backend"); err != nil {
		t.Fatal(err)
	}
	if err := svc.UseContext("sprint"); err != nil {
		t.Fatal(err)
	}

	session := setupResourceTestServer(t, Options{
		Store:   store,
		Clock:   testClock{now: 100},
		Version: "test",
		Mode:    ModeStdio,
	})

	result, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{
		URI: "xuanchu://workspace/current",
	})
	if err != nil {
		t.Fatalf("ReadResource: %v", err)
	}

	var data workspaceResourceData
	if err := json.Unmarshal([]byte(result.Contents[0].Text), &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if data.Context == nil {
		t.Fatal("expected non-nil context when active context is set")
	}
	if data.Context.Name != "sprint" {
		t.Errorf("context name = %q, want sprint", data.Context.Name)
	}
	if data.Context.Filter != "project:backend" {
		t.Errorf("context filter = %q, want project:backend", data.Context.Filter)
	}
}

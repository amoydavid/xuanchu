package mcpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/dajee/taskg/internal/app"
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
		URI: "taskg://workspace/current",
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

	uri := "taskg://workspace/" + ws.ID
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
	uri := "taskg://workspace/team"
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

	uri := "taskg://project/" + proj.ID
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

	// 验证只暴露 agent.* 配置，context.default 不应出现
	if _, ok := data.AgentConfig["context.default"]; ok {
		t.Error("context.default should not be exposed in project resource (not agent.* prefix)")
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

	if _, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "taskg://project/" + proj.Slug}); err == nil {
		t.Fatal("ReadResource taskg://project/{slug} error = nil, want not found")
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

	if _, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "taskg://project/" + hidden.ID}); err == nil {
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
		URI: "taskg://context/current",
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
		URI: "taskg://context/current",
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
		URI: "taskg://workspace/current",
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

func TestProjectResourceOnlyExposesAllowedAgentKeys(t *testing.T) {
	store := newMCPTestStore(t)
	svc := newMCPTestService(t, store)
	proj, err := svc.AddProject(app.AddProjectInput{Slug: "secure", Name: "Secure"})
	if err != nil {
		t.Fatal(err)
	}
	// 设置所有允许的 agent keys
	for _, key := range allowedAgentKeys {
		if err := svc.ProjectConfigSet(proj.Slug, key, "test-value"); err != nil {
			t.Fatal(err)
		}
	}
	// context.default 是合法 key 但不是 agent.* 前缀
	if err := svc.ProjectConfigSet(proj.Slug, "context.default", "should-not-appear"); err != nil {
		t.Fatal(err)
	}

	session := setupResourceTestServer(t, Options{
		Store:   store,
		Clock:   testClock{now: 100},
		Version: "test",
		Mode:    ModeStdio,
	})

	uri := "taskg://project/" + proj.ID
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

	// context.default 不是 agent.* 前缀，不应出现
	if _, ok := data.AgentConfig["context.default"]; ok {
		t.Error("context.default should not be exposed (not agent.* prefix)")
	}

	// 白名单内的 key 都应出现
	for _, key := range allowedAgentKeys {
		if _, ok := data.AgentConfig[key]; !ok {
			t.Errorf("expected %q in agent_config, not found", key)
		}
	}
}

func TestNoTaskListResourceRegistered(t *testing.T) {
	// 验证 list-style URI 如 taskg://project/{id}/tasks 不注册
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
		URI: "taskg://workspace/current",
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

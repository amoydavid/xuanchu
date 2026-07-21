package mcpserver

import (
	"context"
	"encoding/json"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"

	"git.dajee.net/dajee/xuanchu/internal/app"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMCPProjectTemplateSchemasRequireExplicitWorkspaceAndCurrentSnapshot(t *testing.T) {
	assertRequired := func(t *testing.T, required []string, want ...string) {
		t.Helper()
		for _, field := range want {
			if !slices.Contains(required, field) {
				t.Fatalf("required = %v, missing %q", required, field)
			}
		}
	}

	listSchema := mustSchema[ProjectTemplateListInput](t)
	assertRequired(t, listSchema.Required, "workspace")
	if got := listSchema.Properties["workspace"].Description; !strings.Contains(got, "required for every template call") {
		t.Fatalf("list workspace description = %q", got)
	}

	instantiateSchema := mustSchema[ProjectTemplateInstantiateInput](t)
	assertRequired(t, instantiateSchema.Required,
		"workspace", "template", "snapshot_id", "expected_snapshot_hash",
		"project_slug", "project_name", "start_date",
	)
	if got := instantiateSchema.Properties["workspace"].Description; !strings.Contains(got, "required for every template call") {
		t.Fatalf("instantiate workspace description = %q", got)
	}
}

func TestMCPProjectTemplateToolsAreExactlyNarrowSurface(t *testing.T) {
	srv := NewServer(Options{Version: "test"})
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	go func() {
		if err := srv.Run(context.Background(), serverTransport); err != nil {
			t.Logf("server run: %v", err)
		}
	}()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, err := client.Connect(context.Background(), clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer session.Close()

	result, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	var names []string
	for _, tool := range result.Tools {
		if strings.HasPrefix(tool.Name, "project_template_") {
			names = append(names, tool.Name)
		}
	}
	sort.Strings(names)
	want := []string{"project_template_instantiate", "project_template_list"}
	if !slices.Equal(names, want) {
		t.Fatalf("project template tools = %v, want exactly %v", names, want)
	}
}

func TestMCPProjectTemplateListReturnsOnlyActiveCurrentSummaries(t *testing.T) {
	srv, svc := newMCPProjectTemplateFixture(t)
	active := seedMCPProjectTemplate(t, svc, "launch", "tplsource")
	archived := seedMCPProjectTemplate(t, svc, "retired", "oldsource")
	if _, err := svc.ArchiveProjectTemplate(archived.Template.Key); err != nil {
		t.Fatalf("archive template: %v", err)
	}

	result := callTool(t, connectClient(t, srv), "project_template_list", ProjectTemplateListInput{
		Workspace: "local", Q: "", Limit: 20, Offset: 0,
	})
	if result.IsError {
		t.Fatalf("project_template_list error: %#v", result.StructuredContent)
	}
	assertProjectTemplateEnvelopeEquivalent(t, result)
	data := envelopeData(t, parseEnvelope(t, result))
	items := nestedSlice(t, data, "items")
	if len(items) != 1 {
		t.Fatalf("items = %#v, want one active template", items)
	}
	item := items[0].(map[string]any)
	if item["key"] != active.Template.Key || item["status"] != "active" {
		t.Fatalf("template summary = %#v", item)
	}
	createdBy := nestedMap(t, item, "created_by")
	createdByUser := nestedMap(t, createdBy, "user")
	if createdBy["type"] != "user" || createdByUser["id"] == "" || createdByUser["name"] == "" {
		t.Fatalf("created_by 未使用统一 actor/user JSON: %#v", createdBy)
	}
	current := nestedMap(t, item, "current_snapshot")
	if current["id"] != active.Template.CurrentSnapshot.ID || current["hash"] != active.Template.CurrentSnapshot.Hash {
		t.Fatalf("current snapshot = %#v", current)
	}
	currentCreatedBy := nestedMap(t, current, "created_by")
	if currentCreatedBy["type"] != "user" || nestedMap(t, currentCreatedBy, "user")["id"] == "" {
		t.Fatalf("current created_by 未使用统一 actor/user JSON: %#v", currentCreatedBy)
	}
	raw, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"retired", "snapshot_json", "versions"} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("list exposed %q: %s", forbidden, raw)
		}
	}
}

func TestMCPProjectTemplateInstantiateUsesCurrentSnapshotEnvelope(t *testing.T) {
	srv, svc := newMCPProjectTemplateFixture(t)
	created := seedMCPProjectTemplate(t, svc, "launch", "tplsource")
	current := created.Template.CurrentSnapshot

	result := callTool(t, connectClient(t, srv), "project_template_instantiate", ProjectTemplateInstantiateInput{
		Workspace: "local", Template: "launch", SnapshotID: current.ID,
		ExpectedSnapshotHash: current.Hash, ProjectSlug: "newproj", ProjectName: "新项目", StartDate: "2026-08-01",
	})
	if result.IsError {
		t.Fatalf("project_template_instantiate error: %#v", result.StructuredContent)
	}
	assertProjectTemplateEnvelopeEquivalent(t, result)
	data := envelopeData(t, parseEnvelope(t, result))
	project := nestedMap(t, data, "project")
	if project["slug"] != "newproj" || project["name"] != "新项目" {
		t.Fatalf("project = %#v", project)
	}
	counts := nestedMap(t, data, "counts")
	for _, field := range []string{"configs", "tasks", "series", "automations"} {
		if counts[field] != float64(0) {
			t.Fatalf("counts.%s = %#v, want 0", field, counts[field])
		}
	}
}

func TestMCPProjectTemplateValidationErrorKeepsIssuesWithoutSecret(t *testing.T) {
	srv, svc := newMCPProjectTemplateFixture(t)
	created := seedMCPProjectTemplate(t, svc, "launch", "tplsource")
	current := created.Template.CurrentSnapshot
	secret := "sk-mcp-project-template-must-not-leak"

	result := callTool(t, connectClient(t, srv), "project_template_instantiate", ProjectTemplateInstantiateInput{
		Workspace: "local", Template: "launch", SnapshotID: current.ID,
		ExpectedSnapshotHash: current.Hash, ProjectSlug: "tplsource", ProjectName: "重复项目", StartDate: "2026-08-01",
		SecretInputs: map[string]string{"unused.secret": secret},
	})
	if !result.IsError {
		t.Fatal("project_template_instantiate succeeded, want validation error")
	}
	raw, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var toolErr ProjectTemplateToolError
	if err := json.Unmarshal(raw, &toolErr); err != nil {
		t.Fatal(err)
	}
	if toolErr.Code != "project_already_exists" || len(toolErr.Issues) == 0 || toolErr.Issues[0].Code != "project_already_exists" {
		t.Fatalf("structured error = %#v", toolErr)
	}
	if strings.Contains(string(raw), secret) || strings.Contains(renderedText(result), secret) {
		t.Fatalf("validation error leaked secret: structured=%s text=%q", raw, renderedText(result))
	}
}

func TestMCPProjectTemplateInstantiateRejectsStaleSnapshotAndBlankWorkspace(t *testing.T) {
	srv, svc := newMCPProjectTemplateFixture(t)
	created := seedMCPProjectTemplate(t, svc, "launch", "tplsource")
	current := created.Template.CurrentSnapshot
	session := connectClient(t, srv)

	blankWorkspace := callTool(t, session, "project_template_list", ProjectTemplateListInput{Workspace: ""})
	if got := parseError(t, blankWorkspace).Code; got != "workspace_required" {
		t.Fatalf("blank workspace code = %q, want workspace_required", got)
	}

	secret := "sk-stale-must-not-leak"
	stale := callTool(t, session, "project_template_instantiate", ProjectTemplateInstantiateInput{
		Workspace: "local", Template: "launch", SnapshotID: "stale-snapshot",
		ExpectedSnapshotHash: current.Hash, ProjectSlug: "newproj", ProjectName: "新项目", StartDate: "2026-08-01",
		SecretInputs: map[string]string{"unused.secret": secret},
	})
	if got := parseError(t, stale).Code; got != "project_template_snapshot_hash_mismatch" {
		t.Fatalf("stale snapshot code = %q, want project_template_snapshot_hash_mismatch", got)
	}
	if strings.Contains(renderedText(stale), secret) {
		t.Fatalf("stale error leaked secret: %q", renderedText(stale))
	}
}

func TestMCPProjectTemplateListUsesExplicitWorkspaceIsolation(t *testing.T) {
	srv, svc := newMCPProjectTemplateFixture(t)
	seedMCPProjectTemplate(t, svc, "launch", "tplsource")
	if _, err := svc.AddWorkspace(app.AddWorkspaceInput{Slug: "other", Name: "其他空间"}); err != nil {
		t.Fatalf("add workspace: %v", err)
	}

	result := callTool(t, connectClient(t, srv), "project_template_list", ProjectTemplateListInput{Workspace: "other"})
	if result.IsError {
		t.Fatalf("project_template_list other workspace error: %#v", result.StructuredContent)
	}
	data := envelopeData(t, parseEnvelope(t, result))
	if items := nestedSlice(t, data, "items"); len(items) != 0 {
		t.Fatalf("other workspace items = %#v, want empty", items)
	}
}

func newMCPProjectTemplateFixture(t *testing.T) (*mcp.Server, *app.Service) {
	t.Helper()
	srv, store := newTestServerWithOptions(t, Options{})
	svc, err := app.NewService(app.ServiceOptions{Store: store, Clock: fixedTestClock(), ActorRef: "local", WorkspaceRef: "local"})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return srv, svc
}

func seedMCPProjectTemplate(t *testing.T, svc *app.Service, key, sourceProject string) app.ProjectTemplateView {
	t.Helper()
	if _, err := svc.AddProject(app.AddProjectInput{Slug: sourceProject, Name: "模板来源"}); err != nil {
		t.Fatalf("add source project: %v", err)
	}
	input := app.CaptureInput{
		SourceProjectRef: sourceProject,
		AnchorDate:       "2026-07-20",
		Selection: app.CaptureSelection{
			ConfigKeys: []string{}, TaskRefs: []string{}, SeriesRefs: []string{}, AutomationRuleIDs: []string{},
		},
		SelectionPresence: app.SelectionPresence{ConfigKeys: true, TaskRefs: true, SeriesRefs: true, AutomationRuleIDs: true},
	}
	preview, err := svc.PreviewProjectTemplateCapture(input)
	if err != nil {
		t.Fatalf("preview capture: %v", err)
	}
	input.ExpectedSourceHash = preview.SourceHash
	created, err := svc.CreateProjectTemplate(app.CreateTemplateInput{Key: key, Name: "项目模板 " + key, Description: "MCP 测试模板", Capture: input})
	if err != nil {
		t.Fatalf("create template: %v", err)
	}
	return created
}

func assertProjectTemplateEnvelopeEquivalent(t *testing.T, result *mcp.CallToolResult) {
	t.Helper()
	var fromText, fromStructured any
	if err := json.Unmarshal([]byte(renderedText(result)), &fromText); err != nil {
		t.Fatalf("text is not JSON ToolEnvelope: %v\ntext=%q", err, renderedText(result))
	}
	raw, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &fromStructured); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(fromText, fromStructured) {
		t.Fatalf("text/structuredContent diverged:\ntext=%#v\nstructured=%#v", fromText, fromStructured)
	}
}

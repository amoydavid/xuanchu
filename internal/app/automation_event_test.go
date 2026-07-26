package app

import (
	"encoding/json"
	"strings"
	"testing"

	"git.dajee.net/dajee/xuanchu/internal/storage"
)

// defineWorkspaceProjectCreatedRuleForTest 创建一条 enabled 的 Workspace project.created 规则。
func defineWorkspaceProjectCreatedRuleForTest(t *testing.T, svc *Service, name string) AutomationRuleView {
	t.Helper()
	rule, err := svc.AddWorkspaceAutomationRule(AutomationRuleInput{
		Name:                name,
		Enabled:             true,
		TriggerType:         ProjectAutomationTriggerEvent,
		TriggerConfig:       ProjectAutomationTriggerConfig{EventType: AutomationEventTypeProjectCreated},
		Action:              defaultAutomationActionForTest(),
		Context:             ProjectAutomationContextConfig{Include: []string{"workspace", "project", "project_config", "event"}},
		InstructionTemplate: "初始化知识库",
	})
	if err != nil {
		t.Fatalf("AddWorkspaceAutomationRule: %v", err)
	}
	return rule
}

func TestProjectCreatedAutomationEnqueuesDeliveryInSameTransaction(t *testing.T) {
	f := newWorkspaceAutomationFixture(t)
	defineWorkspaceProviderConfigForTest(t, f.svc, "https://agent.example.com")
	defineWorkspaceProjectCreatedRuleForTest(t, f.svc, "新项目知识库初始化")

	project, err := f.svc.AddProject(AddProjectInput{Slug: "adsops", Name: "广告投放"})
	if err != nil {
		t.Fatalf("AddProject: %v", err)
	}
	deliveries, err := storage.NewAutomationDeliveryRepository(f.store.DB()).List(storage.AutomationDeliveryListOptions{
		WorkspaceID: f.svc.workspaceID,
	})
	if err != nil {
		t.Fatalf("List deliveries: %v", err)
	}
	if len(deliveries) != 1 {
		t.Fatalf("deliveries = %#v, want 1", deliveries)
	}
	d := deliveries[0]
	if d.RuleScopeType != storage.AutomationScopeWorkspace || d.ProjectID == nil || *d.ProjectID != project.ID {
		t.Fatalf("delivery scope/project = %#v", d)
	}
	if d.Status != storage.DeliveryStatusQueued {
		t.Fatalf("status = %q, want queued", d.Status)
	}
	if d.EventID == "" || d.EventType != AutomationEventTypeProjectCreated {
		t.Fatalf("event fields missing: %#v", d)
	}
	// 同 rule + event_id 重复路由（模拟 dispatcher/路由重试）只产生一条。
	// project.created 只在 Project 创建时产生一次，因此 dedupe 必须以 event_id 为准。
	actor := HookActorSnapshot{ActorType: "user", ActorUserID: f.svc.runtime.ActorUserID}
	event := buildProjectCreatedAutomationEvent(
		storage.Project{ID: project.ID, WorkspaceID: f.svc.workspaceID},
		actor, map[string]any{"source": "empty"}, f.clock.Unix(),
	)
	// 复用第一次 route 产生的 event_id：通过 list 找到第一条 delivery 上的 event_id。
	firstDelivery := deliveries[0]
	event.EventID = firstDelivery.EventID
	snapshot, err := f.svc.buildProjectCreatedContextSnapshot(storage.Project{ID: project.ID, WorkspaceID: f.svc.workspaceID, Slug: project.Slug, Name: project.Name, Status: project.Status})
	if err != nil {
		t.Fatalf("buildProjectCreatedContextSnapshot: %v", err)
	}
	if err := f.svc.RouteAutomationEventTx(event, snapshot); err != nil {
		t.Fatalf("re-route same event_id: %v", err)
	}
	deliveries, _ = storage.NewAutomationDeliveryRepository(f.store.DB()).List(storage.AutomationDeliveryListOptions{WorkspaceID: f.svc.workspaceID})
	if len(deliveries) != 1 {
		t.Fatalf("dedupe by event_id failed, got %d", len(deliveries))
	}
}

func TestProjectCreatedAutomationDoesNotEnqueueWhenNoRule(t *testing.T) {
	f := newWorkspaceAutomationFixture(t)
	defineWorkspaceProviderConfigForTest(t, f.svc, "https://agent.example.com")
	if _, err := f.svc.AddProject(AddProjectInput{Slug: "adsops", Name: "广告投放"}); err != nil {
		t.Fatalf("AddProject: %v", err)
	}
	deliveries, err := storage.NewAutomationDeliveryRepository(f.store.DB()).List(storage.AutomationDeliveryListOptions{WorkspaceID: f.svc.workspaceID})
	if err != nil {
		t.Fatal(err)
	}
	if len(deliveries) != 0 {
		t.Fatalf("deliveries should be empty without rule, got %#v", deliveries)
	}
}

func TestProjectCreatedAutomationDeadLettersOnMissingProviderConfig(t *testing.T) {
	f := newWorkspaceAutomationFixture(t)
	// 不定义 Provider config，规则运行错误必须落 dead_lettered，不阻断 Project 创建。
	defineWorkspaceProjectCreatedRuleForTest(t, f.svc, "缺 Provider 规则")

	project, err := f.svc.AddProject(AddProjectInput{Slug: "adsops", Name: "广告投放"})
	if err != nil {
		t.Fatalf("AddProject should still succeed: %v", err)
	}
	deliveries, err := storage.NewAutomationDeliveryRepository(f.store.DB()).List(storage.AutomationDeliveryListOptions{WorkspaceID: f.svc.workspaceID})
	if err != nil {
		t.Fatal(err)
	}
	if len(deliveries) != 1 || deliveries[0].Status != storage.DeliveryStatusDeadLettered {
		t.Fatalf("deliveries = %#v, want 1 dead_lettered", deliveries)
	}
	d := deliveries[0]
	if !strings.Contains(d.LastError, "automation_provider_config_missing") {
		t.Fatalf("last_error should contain stable code: %q", d.LastError)
	}
	// Project 仍创建成功。
	if project.ID == "" {
		t.Fatalf("project not created")
	}
}

func TestProjectCreatedAutomationProjectContextExcludesSecret(t *testing.T) {
	f := newWorkspaceAutomationFixture(t)
	defineWorkspaceProviderConfigForTest(t, f.svc, "https://agent.example.com")
	// 在 Workspace 写入一个 secret config 和一个非 secret config。
	defineWorkspaceConfigForTest(t, f.svc, "feishu.drive_folder_token", false, "fldcnWorkspace")
	defineWorkspaceConfigForTest(t, f.svc, "agent.runtime.secret", true, "should-not-leak")
	defineWorkspaceProjectCreatedRuleForTest(t, f.svc, "知识库初始化")

	if _, err := f.svc.AddProject(AddProjectInput{Slug: "adsops", Name: "广告投放"}); err != nil {
		t.Fatalf("AddProject: %v", err)
	}
	deliveries, _ := storage.NewAutomationDeliveryRepository(f.store.DB()).List(storage.AutomationDeliveryListOptions{WorkspaceID: f.svc.workspaceID})
	if len(deliveries) != 1 {
		t.Fatalf("deliveries = %#v", deliveries)
	}
	body := deliveries[0].RequestBodyJSON
	if strings.Contains(body, "should-not-leak") {
		t.Fatalf("request body leaked secret: %s", body)
	}
	if !strings.Contains(body, "feishu.drive_folder_token") {
		t.Fatalf("request body missing non-secret config: %s", body)
	}
	// Metadata source=empty，无 template 字段。
	if !strings.Contains(body, `"source":"empty"`) {
		t.Fatalf("metadata source not empty: %s", body)
	}
}

func TestProjectCreatedAutomationTemplateMetadataAndCounts(t *testing.T) {
	f := newWorkspaceAutomationFixture(t)
	defineWorkspaceProviderConfigForTest(t, f.svc, "https://agent.example.com")
	defineWorkspaceProjectCreatedRuleForTest(t, f.svc, "知识库初始化")

	// 直接调用 routeProjectCreatedAutomation，模拟 Template Instantiate 结束后的状态。
	// 这里只验证 metadata 字段被正确写入 frozen request body。
	project := storage.Project{ID: "proj-tpl", WorkspaceID: f.svc.workspaceID, Slug: "inst", Name: "实例化项目", Status: "planning"}
	// 必须先在 DB 创建 project + workspace row，才能让 buildProjectCreatedContextSnapshot 拿到数据。
	ws, err := f.store.LocalWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	if err := f.store.DB().Create(&project).Error; err != nil {
		t.Fatal(err)
	}
	// 用临时 service 调用 route（避免依赖完整 instantiate）。
	metadata := map[string]any{
		"source":                           "template",
		"source_template_id":               "tpl-id",
		"source_template_snapshot_id":      "snap-id",
		"source_template_snapshot_hash":    "snap-hash",
		"initial_task_count":               3,
		"initial_series_count":             1,
		"initial_project_automation_count": 1,
	}
	if err := f.svc.routeProjectCreatedAutomation(project, metadata); err != nil {
		t.Fatalf("routeProjectCreatedAutomation: %v", err)
	}
	deliveries, _ := storage.NewAutomationDeliveryRepository(f.store.DB()).List(storage.AutomationDeliveryListOptions{WorkspaceID: ws.ID})
	if len(deliveries) != 1 {
		t.Fatalf("deliveries = %#v, want 1", deliveries)
	}
	body := deliveries[0].RequestBodyJSON
	if !strings.Contains(body, `"source":"template"`) {
		t.Fatalf("metadata source not template: %s", body)
	}
	if !strings.Contains(body, `"source_template_id":"tpl-id"`) {
		t.Fatalf("metadata missing template id: %s", body)
	}
	// 解析 metadata 验证 count。count 在 JSON 中是 number，解码到 any 后是 float64。
	var parsed struct {
		Input []struct {
			InputJSON struct {
				Event struct {
					Metadata map[string]any `json:"metadata"`
				} `json:"event"`
			} `json:"input_json"`
		} `json:"input"`
	}
	if err := json.Unmarshal([]byte(body), &parsed); err != nil {
		t.Fatalf("parse body: %v\nbody=%s", err, body)
	}
	if len(parsed.Input) == 0 {
		t.Fatalf("body missing input: %s", body)
	}
	md := parsed.Input[0].InputJSON.Event.Metadata
	if toInt(md["initial_task_count"]) != 3 || toInt(md["initial_series_count"]) != 1 || toInt(md["initial_project_automation_count"]) != 1 {
		t.Fatalf("metadata counts = %#v", md)
	}
}

// toInt 把 JSON 解码出来的 number（float64）或 int 转换为 int。
func toInt(v any) int {
	switch x := v.(type) {
	case float64:
		return int(x)
	case int:
		return x
	case int64:
		return int(x)
	}
	return -9999
}

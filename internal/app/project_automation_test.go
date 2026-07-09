package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"

	"git.dajee.net/dajee/xuanchu/internal/storage"
)

type projectAutomationServiceFixture struct {
	store          *storage.Store
	svc            *Service
	clock          FixedClock
	serviceFactory func(workspaceID string) *Service
}

func newProjectAutomationServiceFixture(t *testing.T) *projectAutomationServiceFixture {
	t.Helper()
	store := newTestStore(t)
	clock := FixedClock{NowUnix: 1000}
	svc, err := NewService(ServiceOptions{Store: store, Clock: clock})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	f := &projectAutomationServiceFixture{store: store, svc: svc, clock: clock}
	f.serviceFactory = func(workspaceID string) *Service {
		next, err := NewService(ServiceOptions{Store: store, Clock: FixedClock{NowUnix: f.clock.NowUnix}, WorkspaceRef: workspaceID})
		if err != nil {
			t.Fatalf("NewService(%s): %v", workspaceID, err)
		}
		return next
	}
	return f
}

func defineConfigForTest(t *testing.T, svc *Service, key string, secret bool, value string) {
	t.Helper()
	valueType := string(ConfigValueTypeString)
	if key == "agent.provider.allowed_hosts" {
		valueType = string(ConfigValueTypeJSON)
	}
	if err := svc.ConfigSchemaSet(ConfigSchemaInput{Key: key, ValueType: valueType, AllowedScopes: []string{string(ConfigAllowedScopeWorkspace), string(ConfigAllowedScopeProject)}, Secret: secret}); err != nil {
		t.Fatalf("ConfigSchemaSet(%s): %v", key, err)
	}
	if err := svc.ProjectConfigSet("adsops", key, value); err != nil {
		t.Fatalf("ProjectConfigSet(%s): %v", key, err)
	}
}

func defineProviderConfigForTest(t *testing.T, svc *Service) {
	t.Helper()
	defineProviderConfigForTestWithBaseURL(t, svc, "https://agent.example.com")
}

func defineProviderConfigForTestWithBaseURL(t *testing.T, svc *Service, baseURL string) {
	t.Helper()
	defineConfigForTest(t, svc, "agent.provider.base_url", false, baseURL)
	defineConfigForTest(t, svc, "agent.provider.api_key", true, "sk-test")
	defineConfigForTest(t, svc, "agent.provider.model", false, "project-operator")
	parsed, err := url.Parse(baseURL)
	if err != nil {
		t.Fatalf("parse base url %q: %v", baseURL, err)
	}
	defineConfigForTest(t, svc, "agent.provider.allowed_hosts", false, `["`+parsed.Hostname()+`"]`)
}

func defaultAutomationActionForTest() ProjectAutomationActionConfig {
	return ProjectAutomationActionConfig{Protocol: "chat_completions", BaseURLConfigKey: "agent.provider.base_url", APIKeyConfigKey: "agent.provider.api_key", ModelConfigKey: "agent.provider.model", Temperature: 0.2}
}

func createWorkspaceMemberForTest(t *testing.T, svc *Service, name string) storage.User {
	t.Helper()
	user := mustCreateUserRecord(t, svc.store, storage.User{ID: uuid.NewString(), Name: name, CreatedAt: 100, ModifiedAt: 100})
	mustUpsertMembershipRecord(t, svc.store, storage.Membership{UserID: user.ID, WorkspaceID: svc.Runtime().WorkspaceID, Role: string(RoleMember), JoinedAt: 100, ModifiedAt: 100})
	return user
}

func mustJSONBytes(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json marshal: %v", err)
	}
	return raw
}

func contains(value string, substr string) bool {
	return strings.Contains(value, substr)
}

func runtimeErrorCode(err error) string {
	var rt RuntimeError
	if errors.As(err, &rt) {
		return rt.Code
	}
	return ""
}

func TestProjectAutomationRuleLifecycleRejectsClosedProjectWrites(t *testing.T) {
	f := newProjectAutomationServiceFixture(t)
	project, err := f.svc.AddProject(AddProjectInput{Slug: "adsops", Name: "广告投放优化"})
	if err != nil {
		t.Fatalf("AddProject: %v", err)
	}
	input := ProjectAutomationRuleAddInput{
		Name:                "每日项目巡检",
		Enabled:             true,
		TriggerType:         "schedule",
		TriggerConfig:       ProjectAutomationTriggerConfig{ScheduleType: "daily_at", ScheduleValue: "09:30", Timezone: "Asia/Shanghai"},
		Condition:           ProjectAutomationCondition{TaskFilter: "status:pending", MaxTasks: 50},
		Action:              ProjectAutomationActionConfig{Protocol: "chat_completions", BaseURLConfigKey: "agent.provider.base_url", APIKeyConfigKey: "agent.provider.api_key", ModelConfigKey: "agent.provider.model", Temperature: 0.2},
		Context:             ProjectAutomationContextConfig{Include: []string{"workspace", "project", "task_summary", "matched_tasks", "project_config"}},
		InstructionTemplate: "生成项目巡检报告",
	}
	created, err := f.svc.AddProjectAutomationRule(project.Slug, input)
	if err != nil {
		t.Fatalf("AddProjectAutomationRule: %v", err)
	}
	if created.ID == "" || created.ProjectID != project.ID || created.TriggerType != "schedule" {
		t.Fatalf("created = %#v", created)
	}
	rows, err := f.svc.ListProjectAutomationRules(project.Slug, false)
	if err != nil {
		t.Fatalf("ListProjectAutomationRules: %v", err)
	}
	if len(rows) != 1 || rows[0].Name != "每日项目巡检" {
		t.Fatalf("rows = %#v", rows)
	}
	if _, err := f.svc.TransitionProject(project.Slug, string(storage.ProjectStatusArchived)); err != nil {
		t.Fatalf("archive project: %v", err)
	}
	_, err = f.svc.AddProjectAutomationRule(project.Slug, input)
	if err == nil || runtimeErrorCode(err) != "project_closed" {
		t.Fatalf("closed project add err = %v", err)
	}
	_, err = f.svc.ModifyProjectAutomationRule(project.Slug, created.ID, ProjectAutomationRuleModifyInput{Name: stringPtr("改名")})
	if err == nil || runtimeErrorCode(err) != "project_closed" {
		t.Fatalf("closed project modify err = %v", err)
	}
}

func TestProjectAutomationPreviewMasksSecretAndBuildsContext(t *testing.T) {
	f := newProjectAutomationServiceFixture(t)
	project, err := f.svc.AddProject(AddProjectInput{Slug: "adsops", Name: "广告投放优化"})
	if err != nil {
		t.Fatalf("AddProject: %v", err)
	}
	defineConfigForTest(t, f.svc, "agent.provider.base_url", false, "https://agent.example.com")
	defineConfigForTest(t, f.svc, "agent.provider.api_key", true, "sk-real-secret")
	defineConfigForTest(t, f.svc, "agent.provider.model", false, "project-operator")
	defineConfigForTest(t, f.svc, "agent.provider.allowed_hosts", false, `["agent.example.com"]`)
	defineConfigForTest(t, f.svc, "feishu.chat_id", false, "oc_xxx")

	view, err := f.svc.PreviewProjectAutomation(project.Slug, ProjectAutomationPreviewInput{
		Name:          "每日项目巡检",
		TriggerType:   "schedule",
		TriggerConfig: ProjectAutomationTriggerConfig{ScheduleType: "daily_at", ScheduleValue: "09:30", Timezone: "Asia/Shanghai"},
		Action:        ProjectAutomationActionConfig{Protocol: "chat_completions", BaseURLConfigKey: "agent.provider.base_url", APIKeyConfigKey: "agent.provider.api_key", ModelConfigKey: "agent.provider.model", Temperature: 0.2},
		Context:       ProjectAutomationContextConfig{Include: []string{"workspace", "project", "project_config"}},
		InstructionTemplate: "请生成项目巡检报告",
	})
	if err != nil {
		t.Fatalf("PreviewProjectAutomation: %v", err)
	}
	if view.URL != "https://agent.example.com/v1/chat/completions" {
		t.Fatalf("url = %q", view.URL)
	}
	if view.Headers["Authorization"] != "Bearer ****" {
		t.Fatalf("Authorization = %q", view.Headers["Authorization"])
	}
	body := string(mustJSONBytes(t, view.Body))
	if contains(body, "sk-real-secret") {
		t.Fatalf("preview leaked secret: %s", body)
	}
	// model 在顶层 body，直接断言；feishu.chat_id 在嵌套 context JSON 中，引号会被转义，分别校验 key 和 value。
	if !contains(body, `"model":"project-operator"`) {
		t.Fatalf("preview body missing model: %s", body)
	}
	if !contains(body, "feishu.chat_id") || !contains(body, "oc_xxx") {
		t.Fatalf("preview body missing project_config: %s", body)
	}
}

// TestProjectAutomationPreviewWorksWithoutAllowedHosts 验证 allowed_hosts 未配置时
// 预览仍可工作（allowed_hosts 是可选项，未配置时跳过 host 校验）。
func TestProjectAutomationPreviewWorksWithoutAllowedHosts(t *testing.T) {
	f := newProjectAutomationServiceFixture(t)
	project, err := f.svc.AddProject(AddProjectInput{Slug: "adsops", Name: "广告投放优化"})
	if err != nil {
		t.Fatalf("AddProject: %v", err)
	}
	// 只配置 base_url / api_key / model，不配置 allowed_hosts。
	defineConfigForTest(t, f.svc, "agent.provider.base_url", false, "https://agent.example.com")
	defineConfigForTest(t, f.svc, "agent.provider.api_key", true, "sk-real-secret")
	defineConfigForTest(t, f.svc, "agent.provider.model", false, "project-operator")

	view, err := f.svc.PreviewProjectAutomation(project.Slug, ProjectAutomationPreviewInput{
		Name:                "每日项目巡检",
		TriggerType:         "schedule",
		TriggerConfig:       ProjectAutomationTriggerConfig{ScheduleType: "daily_at", ScheduleValue: "09:30", Timezone: "Asia/Shanghai"},
		Action:              defaultAutomationActionForTest(),
		Context:             ProjectAutomationContextConfig{Include: []string{"project"}},
		InstructionTemplate: "生成巡检",
	})
	if err != nil {
		t.Fatalf("PreviewProjectAutomation without allowed_hosts: %v", err)
	}
	if view.URL != "https://agent.example.com/v1/chat/completions" {
		t.Fatalf("url = %q", view.URL)
	}
}

func TestProjectAutomationSchedulerEnqueuesDailyRuleOnce(t *testing.T) {
	f := newProjectAutomationServiceFixture(t)
	project, err := f.svc.AddProject(AddProjectInput{Slug: "adsops", Name: "广告投放优化"})
	if err != nil {
		t.Fatalf("AddProject: %v", err)
	}
	rule, err := f.svc.AddProjectAutomationRule(project.Slug, ProjectAutomationRuleAddInput{
		Name:          "每日项目巡检",
		Enabled:       true,
		TriggerType:   "schedule",
		TriggerConfig: ProjectAutomationTriggerConfig{ScheduleType: "daily_at", ScheduleValue: "09:30", Timezone: "Asia/Shanghai"},
		Action:        defaultAutomationActionForTest(),
		Context:       ProjectAutomationContextConfig{Include: []string{"workspace", "project"}},
		InstructionTemplate: "生成巡检",
	})
	if err != nil {
		t.Fatalf("AddProjectAutomationRule: %v", err)
	}
	defineProviderConfigForTest(t, f.svc)
	f.clock.NowUnix = mustUnix(t, "2026-07-08T09:31:00+08:00")
	scheduler := NewProjectAutomationScheduler(ProjectAutomationSchedulerOptions{Store: f.store, Clock: f.clock, ServiceFactory: f.serviceFactory})
	result, err := scheduler.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if result.DeliveriesEnqueued != 1 {
		t.Fatalf("DeliveriesEnqueued = %d", result.DeliveriesEnqueued)
	}
	result, err = scheduler.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce duplicate: %v", err)
	}
	if result.DeliveriesEnqueued != 0 {
		t.Fatalf("duplicate run enqueued %d deliveries for rule %s", result.DeliveriesEnqueued, rule.ID)
	}
}

func TestProjectAutomationEventEnqueueUsesAddedAssignees(t *testing.T) {
	f := newProjectAutomationServiceFixture(t)
	project, _ := f.svc.AddProject(AddProjectInput{Slug: "adsops", Name: "广告投放优化"})
	alice := createWorkspaceMemberForTest(t, f.svc, "alice")
	rule, err := f.svc.AddProjectAutomationRule(project.Slug, ProjectAutomationRuleAddInput{
		Name:          "分配任务后拉群",
		Enabled:       true,
		TriggerType:   "event",
		TriggerConfig: ProjectAutomationTriggerConfig{EventType: "task.assigned"},
		Condition:     ProjectAutomationCondition{OnlyAddedAssignees: true},
		Action:        defaultAutomationActionForTest(),
		Context:       ProjectAutomationContextConfig{Include: []string{"event", "task", "added_assignees", "project"}},
		InstructionTemplate: "处理新增负责人",
	})
	if err != nil {
		t.Fatalf("AddProjectAutomationRule: %v", err)
	}
	_ = rule
	defineProviderConfigForTest(t, f.svc)
	tsk, err := f.svc.Add(AddInput{Title: "调整预算策略", Project: &project.Slug})
	if err != nil {
		t.Fatalf("Add task: %v", err)
	}
	err = f.svc.Modify(tsk.UUID, ModifyInput{AddAssignees: []string{alice.Name}})
	if err != nil {
		t.Fatalf("Modify assignee: %v", err)
	}
	deliveries, err := f.svc.ListProjectAutomationDeliveries(project.Slug, ProjectAutomationDeliveryListInput{RuleID: rule.ID})
	if err != nil {
		t.Fatalf("ListProjectAutomationDeliveries: %v", err)
	}
	if len(deliveries) != 1 {
		t.Fatalf("deliveries = %#v", deliveries)
	}
	if !contains(deliveries[0].RequestBodyPreview, "added_assignees") || !contains(deliveries[0].RequestBodyPreview, alice.ID) {
		t.Fatalf("delivery preview missing added assignee: %s", deliveries[0].RequestBodyPreview)
	}
}

func TestProjectAutomationDispatcherSendsOpenAIRequest(t *testing.T) {
	f := newProjectAutomationServiceFixture(t)
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer sk-test" {
			t.Fatalf("Authorization = %q", r.Header.Get("Authorization"))
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body["model"] != "project-operator" {
			t.Fatalf("model = %#v", body["model"])
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl_123","usage":{"prompt_tokens":10,"completion_tokens":4}}`))
	}))
	defer target.Close()
	project, _ := f.svc.AddProject(AddProjectInput{Slug: "adsops", Name: "广告投放优化"})
	defineProviderConfigForTestWithBaseURL(t, f.svc, target.URL)
	rule, _ := f.svc.AddProjectAutomationRule(project.Slug, ProjectAutomationRuleAddInput{
		Name: "每日项目巡检", Enabled: true, TriggerType: "schedule",
		TriggerConfig: ProjectAutomationTriggerConfig{ScheduleType: "daily_at", ScheduleValue: "09:30", Timezone: "Asia/Shanghai"},
		Action: defaultAutomationActionForTest(), Context: ProjectAutomationContextConfig{Include: []string{"project"}},
		InstructionTemplate: "巡检",
	})
	delivery, err := f.svc.TestProjectAutomationRule(project.Slug, rule.ID)
	if err != nil {
		t.Fatalf("TestProjectAutomationRule: %v", err)
	}
	dispatcher := NewProjectAutomationDispatcher(ProjectAutomationDispatcherOptions{Store: f.store, Clock: f.clock, Client: target.Client(), ServiceFactory: f.serviceFactory})
	result, err := dispatcher.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if result.Succeeded != 1 {
		t.Fatalf("dispatch result = %#v", result)
	}
	got, err := f.svc.ProjectAutomationDeliveryInfo(project.Slug, delivery.ID)
	if err != nil {
		t.Fatalf("ProjectAutomationDeliveryInfo: %v", err)
	}
	if got.Status != "succeeded" || got.ProviderRequestID != "chatcmpl_123" {
		t.Fatalf("delivery = %#v", got)
	}
}

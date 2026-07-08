package app

import (
	"encoding/json"
	"errors"
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
	host := strings.TrimPrefix(strings.TrimPrefix(baseURL, "https://"), "http://")
	host = strings.Split(host, "/")[0]
	defineConfigForTest(t, svc, "agent.provider.allowed_hosts", false, `["`+host+`"]`)
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

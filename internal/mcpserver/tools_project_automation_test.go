package mcpserver

import (
	"testing"

	"git.dajee.net/dajee/xuanchu/internal/app"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// newProjectAutomationMCPFixture 构建注册了全部工具的 MCP server 和 local 种子 service。
func newProjectAutomationMCPFixture(t *testing.T) (*mcp.Server, *app.Service) {
	t.Helper()
	srv, store := newTestServerWithOptions(t, Options{})
	svc, err := app.NewService(app.ServiceOptions{Store: store, Clock: fixedTestClock(), ActorRef: "local", WorkspaceRef: "local"})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	if _, err := svc.AddProject(app.AddProjectInput{Slug: "adsops", Name: "广告投放优化"}); err != nil {
		t.Fatalf("AddProject: %v", err)
	}
	defineProjectAutomationProviderConfig(t, svc)
	return srv, svc
}

// defineProjectAutomationProviderConfig 定义 agent.provider.* config 并写入 project scope。
func defineProjectAutomationProviderConfig(t *testing.T, svc *app.Service) {
	t.Helper()
	set := func(key string, valueType string, secret bool, value string) {
		t.Helper()
		if err := svc.ConfigSchemaSet(app.ConfigSchemaInput{Key: key, ValueType: valueType, AllowedScopes: []string{string(app.ConfigAllowedScopeWorkspace), string(app.ConfigAllowedScopeProject)}, Secret: secret}); err != nil {
			t.Fatalf("ConfigSchemaSet(%s): %v", key, err)
		}
		if err := svc.ProjectConfigSet("adsops", key, value); err != nil {
			t.Fatalf("ProjectConfigSet(%s): %v", key, err)
		}
	}
	set("agent.provider.base_url", string(app.ConfigValueTypeString), false, "https://agent.example.com")
	set("agent.provider.api_key", string(app.ConfigValueTypeString), true, "sk-test")
	set("agent.provider.model", string(app.ConfigValueTypeString), false, "project-operator")
	set("agent.provider.allowed_hosts", string(app.ConfigValueTypeJSON), false, `["agent.example.com"]`)
}

func automationRuleAddInputForTest() ProjectAutomationAddInput {
	return ProjectAutomationAddInput{
		Workspace:      "local",
		Project:        "adsops",
		Name:           "每日站会摘要",
		Description:    "汇总当天任务",
		TriggerType:    "schedule",
		TriggerConfig:  automationTriggerConfigInput{ScheduleType: "daily_at", ScheduleValue: "09:30"},
		Condition:      automationConditionInput{MaxTasks: 10},
		Action:         automationActionInput{BaseURLConfigKey: "agent.provider.base_url", APIKeyConfigKey: "agent.provider.api_key", ModelConfigKey: "agent.provider.model"},
		InstructionTemplate: "汇总 {{project.name}} 的任务：{{task_summary}}",
		SystemPrompt:   "你是项目助理",
	}
}

func automationEventRuleAddInputForTest() ProjectAutomationAddInput {
	in := automationRuleAddInputForTest()
	in.Name = "分派通知"
	in.TriggerType = "event"
	in.TriggerConfig = automationTriggerConfigInput{EventType: "task.assigned"}
	in.Condition = automationConditionInput{OnlyAddedAssignees: true}
	return in
}

func TestProjectAutomationRuleLifecycle(t *testing.T) {
	srv, _ := newProjectAutomationMCPFixture(t)
	session := connectClient(t, srv)

	created := callTool(t, session, "project_automation_add", automationRuleAddInputForTest())
	rule := nestedMap(t, envelopeData(t, parseEnvelope(t, created)), "automation")
	if rule["name"] != "每日站会摘要" || rule["trigger_type"] != "schedule" {
		t.Fatalf("created rule = %#v", rule)
	}
	if rule["enabled"] != true {
		t.Fatalf("enabled 默认应为 true，got %v", rule["enabled"])
	}
	// created_by 必须是 JSONUserInfo 形状，不允许裸 UUID
	createdBy, ok := rule["created_by"].(map[string]any)
	if !ok || createdBy["id"] == nil || createdBy["id"].(string) == "" {
		t.Fatalf("created_by 应为 JSONUserInfo 对象，got %#v", rule["created_by"])
	}
	trigger := nestedMap(t, rule, "trigger_config")
	if trigger["schedule_type"] != "daily_at" || trigger["schedule_value"] != "09:30" {
		t.Fatalf("trigger_config = %#v", trigger)
	}
	ruleID, _ := rule["id"].(string)
	if ruleID == "" {
		t.Fatalf("rule id 为空")
	}

	// list
	listed := callTool(t, session, "project_automation_list", ProjectAutomationListInput{Workspace: "local", Project: "adsops"})
	listData := envelopeData(t, parseEnvelope(t, listed))
	if count, _ := listData["count"].(float64); count != 1 {
		t.Fatalf("count = %v, want 1", listData["count"])
	}

	// modify：改名 + 调整 condition
	newName := "每日站会摘要 v2"
	newMax := 20
	modified := callTool(t, session, "project_automation_modify", ProjectAutomationModifyInput{
		Workspace: "local", Project: "adsops", RuleID: ruleID,
		Name:      &newName,
		Condition: &automationConditionInput{MaxTasks: newMax},
	})
	modRule := nestedMap(t, envelopeData(t, parseEnvelope(t, modified)), "automation")
	if modRule["name"] != newName {
		t.Fatalf("modified name = %v, want %q", modRule["name"], newName)
	}
	modCondition := nestedMap(t, modRule, "condition")
	if max, _ := modCondition["max_tasks"].(float64); max != 20 {
		t.Fatalf("modified max_tasks = %v, want 20", modCondition["max_tasks"])
	}

	// disable / enable
	disabled := callTool(t, session, "project_automation_disable", ProjectAutomationRefInput{Workspace: "local", Project: "adsops", RuleID: ruleID})
	if nestedMap(t, envelopeData(t, parseEnvelope(t, disabled)), "automation")["enabled"] != false {
		t.Fatalf("disable 后 enabled 应为 false")
	}
	enabled := callTool(t, session, "project_automation_enable", ProjectAutomationRefInput{Workspace: "local", Project: "adsops", RuleID: ruleID})
	if nestedMap(t, envelopeData(t, parseEnvelope(t, enabled)), "automation")["enabled"] != true {
		t.Fatalf("enable 后 enabled 应为 true")
	}

	// get
	got := callTool(t, session, "project_automation_get", ProjectAutomationRefInput{Workspace: "local", Project: "adsops", RuleID: ruleID})
	if nestedMap(t, envelopeData(t, parseEnvelope(t, got)), "automation")["name"] != newName {
		t.Fatalf("get 返回名称不符")
	}

	// remove 后 list 为空
	if result := callTool(t, session, "project_automation_remove", ProjectAutomationRefInput{Workspace: "local", Project: "adsops", RuleID: ruleID}); result.IsError {
		parseError(t, result)
	}
	listedAfter := callTool(t, session, "project_automation_list", ProjectAutomationListInput{Workspace: "local", Project: "adsops"})
	if count, _ := envelopeData(t, parseEnvelope(t, listedAfter))["count"].(float64); count != 0 {
		t.Fatalf("remove 后 count 应为 0")
	}
}

func TestProjectAutomationEventRuleWithCondition(t *testing.T) {
	srv, _ := newProjectAutomationMCPFixture(t)
	session := connectClient(t, srv)

	created := callTool(t, session, "project_automation_add", automationEventRuleAddInputForTest())
	rule := nestedMap(t, envelopeData(t, parseEnvelope(t, created)), "automation")
	if rule["trigger_type"] != "event" {
		t.Fatalf("trigger_type = %v, want event", rule["trigger_type"])
	}
	trigger := nestedMap(t, rule, "trigger_config")
	if trigger["event_type"] != "task.assigned" {
		t.Fatalf("event_type = %v, want task.assigned", trigger["event_type"])
	}
	if nestedMap(t, rule, "condition")["only_added_assignees"] != true {
		t.Fatalf("only_added_assignees 应为 true")
	}
}

func TestProjectAutomationAddRejectsInvalidInput(t *testing.T) {
	srv, _ := newProjectAutomationMCPFixture(t)
	session := connectClient(t, srv)

	cases := []struct {
		name string
		in   ProjectAutomationAddInput
	}{
		{"unsupported trigger", func() ProjectAutomationAddInput {
			in := automationRuleAddInputForTest()
			in.TriggerType = "webhook"
			return in
		}()},
		{"missing provider config", func() ProjectAutomationAddInput {
			in := automationRuleAddInputForTest()
			in.Action = automationActionInput{}
			return in
		}()},
		{"missing instruction", func() ProjectAutomationAddInput {
			in := automationRuleAddInputForTest()
			in.InstructionTemplate = ""
			return in
		}()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := callTool(t, session, "project_automation_add", tc.in)
			toolErr := parseError(t, result)
			if toolErr.Code != "automation_rule_invalid" {
				t.Fatalf("code = %q, want automation_rule_invalid (message: %s)", toolErr.Code, toolErr.Message)
			}
		})
	}
}

func TestProjectAutomationRuleNotFound(t *testing.T) {
	srv, _ := newProjectAutomationMCPFixture(t)
	session := connectClient(t, srv)

	result := callTool(t, session, "project_automation_get", ProjectAutomationRefInput{Workspace: "local", Project: "adsops", RuleID: "00000000-0000-0000-0000-000000000000"})
	toolErr := parseError(t, result)
	if toolErr.Code != "automation_rule_not_found" {
		t.Fatalf("code = %q, want automation_rule_not_found", toolErr.Code)
	}
}

func TestProjectAutomationClosedProjectRejectsWrites(t *testing.T) {
	srv, seed := newProjectAutomationMCPFixture(t)
	if _, err := seed.ArchiveProject("adsops"); err != nil {
		t.Fatalf("ArchiveProject: %v", err)
	}
	session := connectClient(t, srv)

	result := callTool(t, session, "project_automation_add", automationRuleAddInputForTest())
	toolErr := parseError(t, result)
	if toolErr.Code != "project_closed" {
		t.Fatalf("code = %q, want project_closed", toolErr.Code)
	}
}

func TestProjectAutomationPreviewMasksSecret(t *testing.T) {
	srv, _ := newProjectAutomationMCPFixture(t)
	session := connectClient(t, srv)

	// 未保存规则预览
	preview := callTool(t, session, "project_automation_preview", automationRuleAddInputForTest())
	previewData := nestedMap(t, envelopeData(t, parseEnvelope(t, preview)), "preview")
	if previewData["method"] != "POST" {
		t.Fatalf("method = %v, want POST", previewData["method"])
	}
	if url, _ := previewData["url"].(string); url != "https://agent.example.com/v1/chat/completions" {
		t.Fatalf("url = %v", previewData["url"])
	}
	headers := nestedMap(t, previewData, "headers")
	if headers["Authorization"] != "Bearer ****" {
		t.Fatalf("Authorization 应脱敏，got %v", headers["Authorization"])
	}
	body := nestedMap(t, previewData, "body")
	if body["model"] != "project-operator" {
		t.Fatalf("model = %v, want project-operator", body["model"])
	}
	messages, ok := body["messages"].([]any)
	if !ok || len(messages) != 2 {
		t.Fatalf("messages = %#v, want system+user", body["messages"])
	}

	// 已保存规则预览
	created := callTool(t, session, "project_automation_add", automationRuleAddInputForTest())
	ruleID, _ := nestedMap(t, envelopeData(t, parseEnvelope(t, created)), "automation")["id"].(string)
	saved := callTool(t, session, "project_automation_preview_saved", ProjectAutomationPreviewSavedInput{Workspace: "local", Project: "adsops", RuleID: ruleID})
	savedData := nestedMap(t, envelopeData(t, parseEnvelope(t, saved)), "preview")
	if savedData["url"] != "https://agent.example.com/v1/chat/completions" {
		t.Fatalf("saved preview url = %v", savedData["url"])
	}
}

func TestProjectAutomationTestDeliveryListReplayFlow(t *testing.T) {
	srv, _ := newProjectAutomationMCPFixture(t)
	session := connectClient(t, srv)

	created := callTool(t, session, "project_automation_add", automationRuleAddInputForTest())
	ruleID, _ := nestedMap(t, envelopeData(t, parseEnvelope(t, created)), "automation")["id"].(string)

	// test 入队 manual_test 投递
	tested := callTool(t, session, "project_automation_test", ProjectAutomationRefInput{Workspace: "local", Project: "adsops", RuleID: ruleID})
	delivery := nestedMap(t, envelopeData(t, parseEnvelope(t, tested)), "delivery")
	if delivery["trigger_type"] != "manual_test" || delivery["status"] != "queued" {
		t.Fatalf("delivery = %#v, want manual_test/queued", delivery)
	}
	deliveryID, _ := delivery["id"].(string)
	if deliveryID == "" {
		t.Fatalf("delivery id 为空")
	}
	renderedHeaders := nestedMap(t, delivery, "rendered_headers")
	if renderedHeaders["Authorization"] == nil {
		t.Fatalf("rendered_headers 缺少 Authorization（masked）")
	}

	// list 可见，且 status 过滤生效
	listed := callTool(t, session, "project_automation_delivery_list", ProjectAutomationDeliveryListInput{Workspace: "local", Project: "adsops"})
	listData := envelopeData(t, parseEnvelope(t, listed))
	if count, _ := listData["count"].(float64); count != 1 {
		t.Fatalf("delivery count = %v, want 1", listData["count"])
	}
	filtered := callTool(t, session, "project_automation_delivery_list", ProjectAutomationDeliveryListInput{Workspace: "local", Project: "adsops", Status: "succeeded"})
	if count, _ := envelopeData(t, parseEnvelope(t, filtered))["count"].(float64); count != 0 {
		t.Fatalf("succeeded 过滤 count = %v, want 0", count)
	}

	// get 单条
	got := callTool(t, session, "project_automation_delivery_get", ProjectAutomationDeliveryRefInput{Workspace: "local", Project: "adsops", DeliveryID: deliveryID})
	if nestedMap(t, envelopeData(t, parseEnvelope(t, got)), "delivery")["id"] != deliveryID {
		t.Fatalf("delivery get 返回不符")
	}

	// replay 生成新的 queued 记录，原记录不变
	replayed := callTool(t, session, "project_automation_delivery_replay", ProjectAutomationDeliveryRefInput{Workspace: "local", Project: "adsops", DeliveryID: deliveryID})
	replayDelivery := nestedMap(t, envelopeData(t, parseEnvelope(t, replayed)), "delivery")
	if replayDelivery["status"] != "queued" || replayDelivery["id"] == deliveryID {
		t.Fatalf("replay delivery = %#v", replayDelivery)
	}
}

func TestProjectAutomationTemplateVars(t *testing.T) {
	srv, _ := newProjectAutomationMCPFixture(t)
	session := connectClient(t, srv)

	result := callTool(t, session, "project_automation_list_template_vars", ProjectAutomationTemplateVarsInput{Workspace: "local", Project: "adsops"})
	data := envelopeData(t, parseEnvelope(t, result))
	triggers := nestedSlice(t, data, "triggers")
	if len(triggers) != 2 {
		t.Fatalf("triggers len = %d, want 2", len(triggers))
	}
	byTrigger := map[string][]any{}
	for _, item := range triggers {
		entry, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("trigger entry type = %T", item)
		}
		byTrigger[entry["trigger"].(string)] = nestedSlice(t, entry, "vars")
	}
	hasVar := func(vars []any, name string) bool {
		for _, v := range vars {
			if m, ok := v.(map[string]any); ok && m["name"] == name {
				return true
			}
		}
		return false
	}
	if !hasVar(byTrigger["schedule"], "task_summary") {
		t.Fatalf("schedule 组缺少 task_summary 变量")
	}
	if !hasVar(byTrigger["event"], "task.title") {
		t.Fatalf("event 组缺少 task.title 变量")
	}
}

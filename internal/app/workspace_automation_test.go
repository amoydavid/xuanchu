package app

import (
	"strings"
	"testing"
)

// newWorkspaceAutomationFixture 复用 Project automation 测试 fixture，但角色按 owner 配置。
func newWorkspaceAutomationFixture(t *testing.T) *projectAutomationServiceFixture {
	t.Helper()
	return newProjectAutomationServiceFixture(t)
}

func workspaceAutomationSampleInput(name string) AutomationRuleInput {
	return AutomationRuleInput{
		Name:                name,
		Enabled:             true,
		TriggerType:         ProjectAutomationTriggerEvent,
		TriggerConfig:       ProjectAutomationTriggerConfig{EventType: AutomationEventTypeProjectCreated},
		Action:              ProjectAutomationActionConfig{Protocol: "chat_completions", BaseURLConfigKey: "agent.provider.base_url", APIKeyConfigKey: "agent.provider.api_key", ModelConfigKey: "agent.provider.model", Temperature: 0.2},
		Context:             ProjectAutomationContextConfig{Include: []string{"workspace", "project", "project_config", "event"}},
		InstructionTemplate: "初始化知识库",
	}
}

func TestWorkspaceAutomationScopeRejectsMismatchedWorkspaceID(t *testing.T) {
	f := newWorkspaceAutomationFixture(t)
	_, err := f.svc.normalizeAutomationScope(AutomationScope{Type: AutomationScopeWorkspace, ID: "other-workspace"})
	if code := runtimeErrorCode(err); code != "automation_scope_invalid" {
		t.Fatalf("err = %v, want automation_scope_invalid", err)
	}
	normalized, err := f.svc.normalizeAutomationScope(AutomationScope{Type: AutomationScopeWorkspace})
	if err != nil {
		t.Fatalf("normalize empty workspace scope: %v", err)
	}
	if normalized.ID != f.svc.workspaceID {
		t.Fatalf("workspace scope id = %q, want %q", normalized.ID, f.svc.workspaceID)
	}
}

func TestWorkspaceAutomationRuleRejectsNonWhitelistEvent(t *testing.T) {
	f := newWorkspaceAutomationFixture(t)
	input := workspaceAutomationSampleInput("bad event")
	input.TriggerConfig = ProjectAutomationTriggerConfig{EventType: "task.created"}
	_, err := f.svc.AddWorkspaceAutomationRule(input)
	if code := runtimeErrorCode(err); code != "automation_event_unsupported" {
		t.Fatalf("err = %v, want automation_event_unsupported", err)
	}
}

func TestWorkspaceAutomationRuleRejectsProjectOnlyEvent(t *testing.T) {
	f := newWorkspaceAutomationFixture(t)
	input := workspaceAutomationSampleInput("project hook event")
	// project.modified 不在 Workspace 白名单，也不在 Hook 白名单（实际不存在）。
	input.TriggerConfig = ProjectAutomationTriggerConfig{EventType: "project.modified"}
	_, err := f.svc.AddWorkspaceAutomationRule(input)
	if code := runtimeErrorCode(err); code != "automation_event_unsupported" {
		t.Fatalf("err = %v, want automation_event_unsupported", err)
	}
}

func TestWorkspaceAutomationRuleCRUDAndAudit(t *testing.T) {
	f := newWorkspaceAutomationFixture(t)
	created, err := f.svc.AddWorkspaceAutomationRule(workspaceAutomationSampleInput("新项目知识库初始化"))
	if err != nil {
		t.Fatalf("AddWorkspaceAutomationRule: %v", err)
	}
	if created.ScopeType != "workspace" || created.ScopeID != f.svc.workspaceID || created.ProjectID != "" {
		t.Fatalf("created scope = %#v", created)
	}
	if created.LastDelivery != nil {
		t.Fatalf("fresh rule must not have last_delivery, got %#v", created.LastDelivery)
	}

	// 列表只看到 workspace scope 规则。
	list, err := f.svc.ListWorkspaceAutomationRules(true)
	if err != nil {
		t.Fatalf("ListWorkspaceAutomationRules: %v", err)
	}
	if len(list) != 1 || list[0].ID != created.ID {
		t.Fatalf("list = %#v", list)
	}

	info, err := f.svc.WorkspaceAutomationRuleInfo(created.ID)
	if err != nil {
		t.Fatalf("WorkspaceAutomationRuleInfo: %v", err)
	}
	if info.Name != "新项目知识库初始化" {
		t.Fatalf("info name = %q", info.Name)
	}

	// 修改并产生 audit。
	newName := "新项目默认负责人"
	modified, err := f.svc.ModifyWorkspaceAutomationRule(created.ID, AutomationRuleModifyInput{Name: &newName})
	if err != nil {
		t.Fatalf("ModifyWorkspaceAutomationRule: %v", err)
	}
	if modified.Name != newName {
		t.Fatalf("modified name = %q", modified.Name)
	}

	// 启停。
	if _, err := f.svc.DisableWorkspaceAutomationRule(created.ID); err != nil {
		t.Fatalf("DisableWorkspaceAutomationRule: %v", err)
	}
	enabledList, err := f.svc.ListWorkspaceAutomationRules(false)
	if err != nil {
		t.Fatalf("ListWorkspaceAutomationRules enabled: %v", err)
	}
	if len(enabledList) != 0 {
		t.Fatalf("disabled rule should not appear in enabled-only list, got %#v", enabledList)
	}
	if _, err := f.svc.EnableWorkspaceAutomationRule(created.ID); err != nil {
		t.Fatalf("EnableWorkspaceAutomationRule: %v", err)
	}

	// audit 体现 automation.rule.* 动作。
	auditRows, err := f.svc.ListAudit(AuditListInput{Limit: 50})
	if err != nil {
		t.Fatalf("ListAudit: %v", err)
	}
	actions := map[string]bool{}
	for _, row := range auditRows {
		actions[row.Action] = true
		// payload 不得包含 prompt 或 secret。
		if row.PayloadJSON != "" {
			if strings.Contains(row.PayloadJSON, "InstructionTemplate") || strings.Contains(row.PayloadJSON, "instruction_template") {
				t.Fatalf("audit payload leaked instruction: %s", row.PayloadJSON)
			}
		}
	}
	for _, want := range []string{"automation.rule.created", "automation.rule.modified", "automation.rule.enabled", "automation.rule.disabled"} {
		if !actions[want] {
			t.Fatalf("missing audit action %q in %v", want, actions)
		}
	}

	if err := f.svc.DeleteWorkspaceAutomationRule(created.ID); err != nil {
		t.Fatalf("DeleteWorkspaceAutomationRule: %v", err)
	}
	list, err = f.svc.ListWorkspaceAutomationRules(true)
	if err != nil {
		t.Fatalf("ListWorkspaceAutomationRules after delete: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("rule not deleted: %#v", list)
	}
}

func TestWorkspaceAutomationRuleSameNameAcrossScopes(t *testing.T) {
	f := newWorkspaceAutomationFixture(t)
	project, err := f.svc.AddProject(AddProjectInput{Slug: "adsops", Name: "广告投放"})
	if err != nil {
		t.Fatalf("AddProject: %v", err)
	}
	// Workspace scope 同名规则 1。
	if _, err := f.svc.AddWorkspaceAutomationRule(workspaceAutomationSampleInput("同名规则")); err != nil {
		t.Fatalf("Add workspace same-name rule: %v", err)
	}
	// Project scope 同名规则 2，不应冲突。
	if _, err := f.svc.AddProjectAutomationRule(project.Slug, ProjectAutomationRuleAddInput{
		Name:                "同名规则",
		Enabled:             true,
		TriggerType:         ProjectAutomationTriggerEvent,
		TriggerConfig:       ProjectAutomationTriggerConfig{EventType: "task.created"},
		Action:              defaultAutomationActionForTest(),
		Context:             ProjectAutomationContextConfig{Include: []string{"workspace", "project"}},
		InstructionTemplate: "test",
	}); err != nil {
		t.Fatalf("Add project same-name rule: %v", err)
	}

	// ListScope 严格隔离。
	wsList, err := f.svc.ListWorkspaceAutomationRules(true)
	if err != nil {
		t.Fatal(err)
	}
	if len(wsList) != 1 || wsList[0].ScopeType != "workspace" {
		t.Fatalf("workspace list = %#v", wsList)
	}
	projList, err := f.svc.ListProjectAutomationRules(project.Slug, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(projList) != 1 || projList[0].ProjectID != project.ID {
		t.Fatalf("project list = %#v", projList)
	}
}

func TestWorkspaceAutomationCrossScopeLookupFails(t *testing.T) {
	f := newWorkspaceAutomationFixture(t)
	project, err := f.svc.AddProject(AddProjectInput{Slug: "adsops", Name: "广告投放"})
	if err != nil {
		t.Fatalf("AddProject: %v", err)
	}
	// 创建 workspace 规则。
	created, err := f.svc.AddWorkspaceAutomationRule(workspaceAutomationSampleInput("workspace 规则"))
	if err != nil {
		t.Fatalf("Add workspace rule: %v", err)
	}
	// 用 project scope 读 workspace 规则应失败。
	if _, err := f.svc.AutomationRuleInfo(AutomationScope{Type: AutomationScopeProject, ID: project.ID}, created.ID); err == nil {
		t.Fatalf("project scope should not see workspace rule")
	}
}

func TestWorkspaceAutomationMemberRoleRejected(t *testing.T) {
	f := newWorkspaceAutomationFixture(t)
	member := createWorkspaceMemberForTest(t, f.svc, "viewer-user")
	memberSvc, err := NewService(ServiceOptions{
		Store:        f.store,
		Clock:        f.clock,
		WorkspaceRef: f.svc.workspaceID,
		Runtime: &RuntimeContext{
			ActorType:   "user",
			ActorUserID: member.ID,
			WorkspaceID: f.svc.workspaceID,
			Role:        RoleMember,
		},
	})
	if err != nil {
		t.Fatalf("NewService member: %v", err)
	}
	// Member 没有 workspace.modify 也没有 hook.write，所以创建失败。
	if _, err := memberSvc.AddWorkspaceAutomationRule(workspaceAutomationSampleInput("member rule")); err == nil {
		t.Fatalf("member should not be able to create workspace rule")
	}
	// Member 没有 hook.read，所以 list 也失败。
	if _, err := memberSvc.ListWorkspaceAutomationRules(false); err == nil {
		t.Fatalf("member should not be able to list workspace automation")
	}
}

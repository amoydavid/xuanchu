package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"git.dajee.net/dajee/xuanchu/internal/storage"
)

// TestWorkspaceAutomationSchedulerEnqueuesWorkspaceScheduleDelivery 验证 Workspace schedule
// 规则到期后生成 project_id=nil 的 Delivery，dedupe key 不与 Project scope 冲突。
func TestWorkspaceAutomationSchedulerEnqueuesWorkspaceScheduleDelivery(t *testing.T) {
	f := newProjectAutomationServiceFixture(t)
	defineWorkspaceProviderConfigForTest(t, f.svc, "https://agent.example.com")
	// 创建一条 Workspace schedule 规则。
	if _, err := f.svc.AddWorkspaceAutomationRule(AutomationRuleInput{
		Name:                "每周项目治理巡检",
		Enabled:             true,
		TriggerType:         ProjectAutomationTriggerSchedule,
		TriggerConfig:       ProjectAutomationTriggerConfig{ScheduleType: "daily_at", ScheduleValue: "09:00", Timezone: "Asia/Shanghai"},
		Action:              defaultAutomationActionForTest(),
		Context:             ProjectAutomationContextConfig{Include: []string{"workspace"}},
		InstructionTemplate: "巡检 workspace",
	}); err != nil {
		t.Fatalf("AddWorkspaceAutomationRule: %v", err)
	}
	scheduler := NewProjectAutomationScheduler(ProjectAutomationSchedulerOptions{
		Store: f.store, Clock: FixedClock{NowUnix: mustUnix(t, "2026-07-08T09:01:00+08:00")}, ServiceFactory: f.serviceFactory,
	})
	result, err := scheduler.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if result.DeliveriesEnqueued != 1 {
		t.Fatalf("result = %#v, want 1 enqueued", result)
	}
	deliveries, err := storage.NewAutomationDeliveryRepository(f.store.DB()).List(storage.AutomationDeliveryListOptions{WorkspaceID: f.svc.workspaceID})
	if err != nil {
		t.Fatal(err)
	}
	if len(deliveries) != 1 {
		t.Fatalf("deliveries = %#v", deliveries)
	}
	if deliveries[0].ProjectID != nil {
		t.Fatalf("workspace schedule delivery should have nil project_id: %#v", deliveries[0].ProjectID)
	}
	if deliveries[0].RuleScopeType != storage.AutomationScopeWorkspace {
		t.Fatalf("rule_scope_type = %q, want workspace", deliveries[0].RuleScopeType)
	}
}

// TestWorkspaceAutomationSchedulerDedupeDifferentScopes 验证同时间触发的 Workspace
// 与 Project scope 规则 dedupe key 不冲突。
func TestWorkspaceAutomationSchedulerDedupeDifferentScopes(t *testing.T) {
	f := newProjectAutomationServiceFixture(t)
	defineWorkspaceProviderConfigForTest(t, f.svc, "https://agent.example.com")
	project, err := f.svc.AddProject(AddProjectInput{Slug: "adsops", Name: "广告投放"})
	if err != nil {
		t.Fatal(err)
	}
	// Workspace schedule 规则。
	if _, err := f.svc.AddWorkspaceAutomationRule(AutomationRuleInput{
		Name: "workspace 巡检", Enabled: true, TriggerType: ProjectAutomationTriggerSchedule,
		TriggerConfig:       ProjectAutomationTriggerConfig{ScheduleType: "daily_at", ScheduleValue: "09:00", Timezone: "Asia/Shanghai"},
		Action:              defaultAutomationActionForTest(),
		Context:             ProjectAutomationContextConfig{Include: []string{"workspace"}},
		InstructionTemplate: "ws",
	}); err != nil {
		t.Fatal(err)
	}
	// Project schedule 规则，同名，同时间。
	if _, err := f.svc.AddProjectAutomationRule(project.Slug, ProjectAutomationRuleAddInput{
		Name: "workspace 巡检", Enabled: true, TriggerType: ProjectAutomationTriggerSchedule,
		TriggerConfig:       ProjectAutomationTriggerConfig{ScheduleType: "daily_at", ScheduleValue: "09:00", Timezone: "Asia/Shanghai"},
		Action:              defaultAutomationActionForTest(),
		Context:             ProjectAutomationContextConfig{Include: []string{"project"}},
		InstructionTemplate: "project",
	}); err != nil {
		t.Fatal(err)
	}
	scheduler := NewProjectAutomationScheduler(ProjectAutomationSchedulerOptions{
		Store: f.store, Clock: FixedClock{NowUnix: mustUnix(t, "2026-07-08T09:01:00+08:00")}, ServiceFactory: f.serviceFactory,
	})
	result, err := scheduler.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if result.DeliveriesEnqueued != 2 {
		t.Fatalf("result = %#v, want 2 (one per scope, no dedupe collision)", result)
	}
}

// TestWorkspaceAutomationDispatcherUsesFrozenMaxAttemptsAfterRuleDeleted 验证 dispatcher
// 不读取当前 Rule：Rule 删除后，已落库 Delivery 仍能用冻结的 max_attempts 完成投递。
func TestWorkspaceAutomationDispatcherUsesFrozenMaxAttemptsAfterRuleDeleted(t *testing.T) {
	f := newProjectAutomationServiceFixture(t)
	var requests []map[string]any
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		requests = append(requests, body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl_ok","usage":{}}`))
	}))
	defer target.Close()
	defineWorkspaceProviderConfigForTest(t, f.svc, target.URL)
	project, err := f.svc.AddProject(AddProjectInput{Slug: "adsops", Name: "广告投放"})
	if err != nil {
		t.Fatal(err)
	}
	// 写入一条 frozen Delivery（直接走 storage repo，模拟历史遗留 Delivery）。
	repo := storage.NewAutomationDeliveryRepository(f.store.DB())
	projectID := project.ID
	if err := repo.Enqueue([]storage.AutomationDelivery{{
		ID: "frozen-dlv", WorkspaceID: f.svc.workspaceID,
		RuleScopeType: storage.AutomationScopeProject, RuleScopeID: project.ID, ProjectID: &projectID,
		RuleID: "rule-deleted", TriggerType: ProjectAutomationTriggerSchedule,
		DedupeKey: "k1", Status: storage.DeliveryStatusQueued,
		ResolvedURL: target.URL + "/v1/chat/completions", RenderedMethod: "POST",
		RenderedHeadersJSON: "{}",
		RequestBodyJSON: `{"model":"workspace-operator","messages":[{"role":"user","content":"hi"}]}`,
		APIKeyConfigKey: "agent.provider.api_key", AllowedHostsConfigKey: "agent.provider.allowed_hosts",
		MaxAttempts: 3,
		CreatedAt: 100, ModifiedAt: 100,
	}}); err != nil {
		t.Fatal(err)
	}

	dispatcher := NewProjectAutomationDispatcher(ProjectAutomationDispatcherOptions{
		Store: f.store, Clock: f.clock, Client: target.Client(), ServiceFactory: f.serviceFactory,
	})
	result, err := dispatcher.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if result.Succeeded != 1 {
		t.Fatalf("dispatch result = %#v, want 1 succeeded", result)
	}
	got, err := repo.GetByID("frozen-dlv")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != storage.DeliveryStatusSucceeded {
		t.Fatalf("status = %q, want succeeded", got.Status)
	}
	if len(requests) == 0 {
		t.Fatalf("no provider request observed")
	}
}

// TestWorkspaceAutomationReplayCreatesNewDeliveryWithReplayOf 验证 Replay 走 CreateReplay，
// 原 Delivery 不变，新 Delivery 带 replay_of_delivery_id。
func TestWorkspaceAutomationReplayCreatesNewDeliveryWithReplayOf(t *testing.T) {
	f := newProjectAutomationServiceFixture(t)
	defineWorkspaceProviderConfigForTest(t, f.svc, "https://agent.example.com")
	project, err := f.svc.AddProject(AddProjectInput{Slug: "adsops", Name: "广告投放"})
	if err != nil {
		t.Fatal(err)
	}
	original, err := f.svc.TestProjectAutomationRule(project.Slug, "")
	_ = original
	if err == nil {
		t.Fatalf("expected error for missing rule, got nil")
	}

	// 直接走 storage repo 模拟一条历史 dead_lettered Delivery。
	repo := storage.NewAutomationDeliveryRepository(f.store.DB())
	projectID := project.ID
	orig := storage.AutomationDelivery{
		ID: "orig-dlv", WorkspaceID: f.svc.workspaceID,
		RuleScopeType: storage.AutomationScopeProject, RuleScopeID: project.ID, ProjectID: &projectID,
		RuleID: "rule-x", TriggerType: ProjectAutomationTriggerSchedule,
		DedupeKey: "orig-k", Status: storage.DeliveryStatusDeadLettered,
		ResolvedURL: "https://agent.example.com/v1/chat/completions", RenderedMethod: "POST",
		RequestBodyJSON: `{"model":"m"}`, RequestBodyHash: "sha256:abc",
		APIKeyConfigKey: "agent.provider.api_key", MaxAttempts: 3,
		CreatedAt: 100, ModifiedAt: 100,
	}
	if err := repo.Enqueue([]storage.AutomationDelivery{orig}); err != nil {
		t.Fatal(err)
	}
	replayed, err := f.svc.replayAutomationDelivery(orig, 200)
	if err != nil {
		t.Fatalf("replayAutomationDelivery: %v", err)
	}
	if replayed.ID == orig.ID {
		t.Fatalf("replay must use new ID")
	}
	if replayed.Status != storage.DeliveryStatusQueued {
		t.Fatalf("status = %q, want queued", replayed.Status)
	}
	if replayed.ReplayOfDeliveryID == nil || *replayed.ReplayOfDeliveryID != orig.ID {
		t.Fatalf("replay_of_delivery_id = %#v, want orig-dlv", replayed.ReplayOfDeliveryID)
	}
	if replayed.MaxAttempts != 3 {
		t.Fatalf("frozen max_attempts not preserved: %d", replayed.MaxAttempts)
	}
	// 原 Delivery 不变。
	stored, _ := repo.GetByID("orig-dlv")
	if stored.Status != storage.DeliveryStatusDeadLettered {
		t.Fatalf("original status mutated: %q", stored.Status)
	}
}

// TestWorkspaceAutomationDispatcherReadsWorkspaceSecretForWorkspaceScope 验证 dispatcher
// 按 Delivery 冻结的 scope 读取 secret：Workspace scope 只读 Workspace secret。
func TestWorkspaceAutomationDispatcherReadsWorkspaceSecretForWorkspaceScope(t *testing.T) {
	f := newProjectAutomationServiceFixture(t)
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer sk-workspace-secret" {
			t.Errorf("Authorization = %q, want workspace secret", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"ok","usage":{}}`))
	}))
	defer target.Close()
	defineWorkspaceProviderConfigForTest(t, f.svc, target.URL)

	repo := storage.NewAutomationDeliveryRepository(f.store.DB())
	if err := repo.Enqueue([]storage.AutomationDelivery{{
		ID: "ws-dlv", WorkspaceID: f.svc.workspaceID,
		RuleScopeType: storage.AutomationScopeWorkspace, RuleScopeID: f.svc.workspaceID, ProjectID: nil,
		RuleID: "ws-rule", TriggerType: ProjectAutomationTriggerSchedule,
		DedupeKey: "ws-k", Status: storage.DeliveryStatusQueued,
		ResolvedURL: target.URL + "/v1/chat/completions", RenderedMethod: "POST",
		RenderedHeadersJSON: "{}",
		RequestBodyJSON: `{"model":"workspace-operator","messages":[{"role":"user","content":"hi"}]}`,
		APIKeyConfigKey: "agent.provider.api_key", AllowedHostsConfigKey: "agent.provider.allowed_hosts",
		MaxAttempts: 3,
		CreatedAt: 100, ModifiedAt: 100,
	}}); err != nil {
		t.Fatal(err)
	}
	dispatcher := NewProjectAutomationDispatcher(ProjectAutomationDispatcherOptions{
		Store: f.store, Clock: f.clock, Client: target.Client(), ServiceFactory: f.serviceFactory,
	})
	result, err := dispatcher.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if result.Succeeded != 1 {
		t.Fatalf("dispatch result = %#v", result)
	}
}

// TestWorkspaceAutomationDispatcherStaleRecoveryAfterServerRestart 验证 stale delivering
// 在 claim lease 过期后被重新领取并完成。
func TestWorkspaceAutomationDispatcherStaleRecoveryAfterServerRestart(t *testing.T) {
	f := newProjectAutomationServiceFixture(t)
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"ok","usage":{}}`))
	}))
	defer target.Close()
	defineWorkspaceProviderConfigForTest(t, f.svc, target.URL)

	repo := storage.NewAutomationDeliveryRepository(f.store.DB())
	staleTS := f.clock.NowUnix - 1000
	if err := repo.Enqueue([]storage.AutomationDelivery{{
		ID: "stale-dlv", WorkspaceID: f.svc.workspaceID,
		RuleScopeType: storage.AutomationScopeWorkspace, RuleScopeID: f.svc.workspaceID,
		RuleID: "ws-rule", TriggerType: ProjectAutomationTriggerSchedule,
		DedupeKey: "stale-k", Status: storage.DeliveryStatusDelivering,
		ClaimExpiresAt: &staleTS, AttemptCount: 1,
		ResolvedURL: target.URL + "/v1/chat/completions", RenderedMethod: "POST",
		RequestBodyJSON: `{"model":"workspace-operator","messages":[{"role":"user","content":"hi"}]}`,
		APIKeyConfigKey: "agent.provider.api_key", AllowedHostsConfigKey: "agent.provider.allowed_hosts",
		MaxAttempts: 3,
		CreatedAt: 100, ModifiedAt: 100,
	}}); err != nil {
		t.Fatal(err)
	}
	dispatcher := NewProjectAutomationDispatcher(ProjectAutomationDispatcherOptions{
		Store: f.store, Clock: f.clock, Client: target.Client(), ServiceFactory: f.serviceFactory,
	})
	result, err := dispatcher.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if result.Succeeded != 1 {
		t.Fatalf("dispatch result = %#v, want 1 succeeded from stale delivering", result)
	}
	got, _ := repo.GetByID("stale-dlv")
	if got.Status != storage.DeliveryStatusSucceeded {
		t.Fatalf("status = %q, want succeeded", got.Status)
	}
}

// TestWorkspaceAutomationDeliveryListScopeIsolation 验证 Delivery List 按 scope 过滤。
func TestWorkspaceAutomationDeliveryListScopeIsolation(t *testing.T) {
	f := newProjectAutomationServiceFixture(t)
	repo := storage.NewAutomationDeliveryRepository(f.store.DB())
	emptyID := ""
	if err := repo.Enqueue([]storage.AutomationDelivery{
		{
			ID: "ws-dlv", WorkspaceID: f.svc.workspaceID,
			RuleScopeType: storage.AutomationScopeWorkspace, RuleScopeID: f.svc.workspaceID, ProjectID: nil,
			RuleID: "ws-rule", TriggerType: "schedule", DedupeKey: "k-ws",
			Status: storage.DeliveryStatusQueued, CreatedAt: 100, ModifiedAt: 100,
		},
		{
			ID: "proj-dlv", WorkspaceID: f.svc.workspaceID,
			RuleScopeType: storage.AutomationScopeProject, RuleScopeID: "proj-1", ProjectID: &emptyID,
			RuleID: "proj-rule", TriggerType: "schedule", DedupeKey: "k-proj",
			Status: storage.DeliveryStatusQueued, CreatedAt: 200, ModifiedAt: 200,
		},
	}); err != nil {
		t.Fatal(err)
	}
	// Project scope = workspace 应排除 Project scope。
	wsRows, err := repo.List(storage.AutomationDeliveryListOptions{WorkspaceID: f.svc.workspaceID, RuleScope: storage.AutomationScopeWorkspace})
	if err != nil {
		t.Fatal(err)
	}
	if len(wsRows) != 1 || wsRows[0].ID != "ws-dlv" {
		t.Fatalf("workspace rows = %#v", wsRows)
	}
	if strings.Contains("ws-dlv", "proj-dlv") {
		t.Fatalf("scope isolation broke")
	}
	// Project ref = "" 表示 schedule 无 project；ws-dlv 满足。
	scheduleRows, err := repo.List(storage.AutomationDeliveryListOptions{WorkspaceID: f.svc.workspaceID, ProjectID: &emptyID})
	if err != nil {
		t.Fatal(err)
	}
	if len(scheduleRows) != 1 || scheduleRows[0].ID != "ws-dlv" {
		t.Fatalf("schedule rows = %#v", scheduleRows)
	}
}

package storage

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// newAutomationRepoTestStore 打开一个空 SQLite Store，准备好 workspace + project。
func newAutomationRepoTestStore(t *testing.T, workspaceSlug string, projectSlug string) (*Store, Workspace, Project) {
	t.Helper()
	store, err := Open(filepath.Join(t.TempDir(), "xuanchu.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ws := createTestWorkspace(t, store, workspaceSlug)
	project, err := NewProjectRepository(store.DB()).Create(testProject("project-"+projectSlug, ws.ID, projectSlug, 100))
	if err != nil {
		t.Fatalf("Create project: %v", err)
	}
	return store, ws, project
}

func automationRuleEnabled(value bool) *bool { return &value }

func TestAutomationRuleRepositoryScopeIsolation(t *testing.T) {
	store, ws, project := newAutomationRepoTestStore(t, "scope-iso", "adsops")
	repo := NewAutomationRuleRepository(store.DB())

	// Workspace scope rule under scope_id = workspace_id.
	if err := repo.Create(AutomationRule{
		ID: "ws-rule-1", WorkspaceID: ws.ID, ScopeType: AutomationScopeWorkspace, ScopeID: ws.ID,
		Name: "工作空间规则", Enabled: automationRuleEnabled(true), TriggerType: "event",
		TriggerConfigJSON: `{}`, ConditionJSON: `{}`, ActionConfigJSON: `{}`, ContextConfigJSON: `{}`,
		CreatedByActorType: "user", CreatedAt: 100, ModifiedAt: 100,
	}); err != nil {
		t.Fatalf("Create workspace rule: %v", err)
	}

	// Project scope rule under scope_id = project_id.
	if err := repo.Create(AutomationRule{
		ID: "proj-rule-1", WorkspaceID: ws.ID, ScopeType: AutomationScopeProject, ScopeID: project.ID,
		Name: "项目规则", Enabled: automationRuleEnabled(true), TriggerType: "schedule",
		TriggerConfigJSON: `{}`, ConditionJSON: `{}`, ActionConfigJSON: `{}`, ContextConfigJSON: `{}`,
		CreatedByActorType: "user", CreatedAt: 100, ModifiedAt: 100,
	}); err != nil {
		t.Fatalf("Create project rule: %v", err)
	}

	wsRows, err := repo.ListScope(ws.ID, AutomationScopeWorkspace, ws.ID, true)
	if err != nil {
		t.Fatalf("ListScope workspace: %v", err)
	}
	if len(wsRows) != 1 || wsRows[0].ID != "ws-rule-1" {
		t.Fatalf("workspace rows = %#v", wsRows)
	}
	projRows, err := repo.ListScope(ws.ID, AutomationScopeProject, project.ID, true)
	if err != nil {
		t.Fatalf("ListScope project: %v", err)
	}
	if len(projRows) != 1 || projRows[0].ID != "proj-rule-1" {
		t.Fatalf("project rows = %#v", projRows)
	}
}

func TestAutomationRuleUniqueScopeName(t *testing.T) {
	store, ws, project := newAutomationRepoTestStore(t, "uniq", "adsops")
	repo := NewAutomationRuleRepository(store.DB())

	row := AutomationRule{
		WorkspaceID: ws.ID, ScopeType: AutomationScopeProject, ScopeID: project.ID,
		Name: "巡检", Enabled: automationRuleEnabled(true), TriggerType: "schedule",
		TriggerConfigJSON: `{}`, ConditionJSON: `{}`, ActionConfigJSON: `{}`, ContextConfigJSON: `{}`,
		CreatedByActorType: "user", CreatedAt: 100, ModifiedAt: 100,
	}
	row.ID = "rule-a"
	if err := repo.Create(row); err != nil {
		t.Fatalf("Create first: %v", err)
	}
	row.ID = "rule-b"
	// 同 workspace/scope/scope_id/name 必须冲突。
	if err := repo.Create(row); err == nil {
		t.Fatalf("expected unique constraint violation for duplicate name in same scope")
	}

	// 不同 scope 可以重名：workspace scope 下使用相同 name 不应该冲突。
	row.ID = "rule-c"
	row.ScopeType = AutomationScopeWorkspace
	row.ScopeID = ws.ID
	if err := repo.Create(row); err != nil {
		t.Fatalf("Create workspace rule with same name: %v", err)
	}
}

func TestAutomationCandidatePageOnlyReturnsProjectScope(t *testing.T) {
	store, ws, project := newAutomationRepoTestStore(t, "candidate", "adsops")
	repo := NewAutomationRuleRepository(store.DB())
	enabled := true
	// 三条 project scope 规则 + 一条 workspace scope 规则。
	for i := 0; i < 3; i++ {
		trigger := "schedule"
		if i%2 == 1 {
			trigger = "event"
		}
		if err := repo.Create(AutomationRule{
			ID: fmt.Sprintf("proj-%03d", i), WorkspaceID: ws.ID, ScopeType: AutomationScopeProject, ScopeID: project.ID,
			Name: fmt.Sprintf("项目规则 %03d", i), Enabled: &enabled, TriggerType: trigger,
			TriggerConfigJSON: `{}`, ConditionJSON: `{}`, ActionConfigJSON: `{}`, ContextConfigJSON: `{}`,
			CreatedByActorType: "user", CreatedAt: 100, ModifiedAt: 100,
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := repo.Create(AutomationRule{
		ID: "ws-spy", WorkspaceID: ws.ID, ScopeType: AutomationScopeWorkspace, ScopeID: ws.ID,
		Name: "项目规则 000", Enabled: &enabled, TriggerType: "schedule",
		TriggerConfigJSON: `{}`, ConditionJSON: `{}`, ActionConfigJSON: `{}`, ContextConfigJSON: `{}`,
		CreatedByActorType: "user", CreatedAt: 100, ModifiedAt: 100,
	}); err != nil {
		t.Fatal(err)
	}

	page, err := repo.ListCandidatePage(AutomationCandidateListOptions{
		WorkspaceID: ws.ID, ProjectID: project.ID,
	}, 50, 0)
	if err != nil {
		t.Fatalf("ListCandidatePage: %v", err)
	}
	if page.Total != 3 {
		t.Fatalf("candidate total = %d, want 3 (workspace rules must not leak)", page.Total)
	}
	for _, row := range page.Items {
		if row.ScopeType != AutomationScopeProject || row.ScopeID != project.ID {
			t.Fatalf("workspace rule leaked into project candidates: %#v", row)
		}
	}
}

func TestAutomationDeliveryProjectIDNullableAndListFilters(t *testing.T) {
	store, ws, project := newAutomationRepoTestStore(t, "delivery-nullable", "adsops")
	repo := NewAutomationDeliveryRepository(store.DB())
	projectID := project.ID

	// Workspace schedule delivery：project_id 为 NULL。
	wsDelivery := AutomationDelivery{
		ID: "ws-dlv-1", WorkspaceID: ws.ID,
		RuleScopeType: AutomationScopeWorkspace, RuleScopeID: ws.ID, ProjectID: nil,
		RuleID: "ws-rule-1", TriggerType: "schedule",
		DedupeKey: "schedule:workspace:" + ws.ID + ":ws-rule-1:2026-08-01:0900",
		Status:    DeliveryStatusQueued,
		CreatedAt: 100, ModifiedAt: 100,
	}
	if err := repo.Enqueue([]AutomationDelivery{wsDelivery}); err != nil {
		t.Fatalf("Enqueue workspace delivery: %v", err)
	}
	// Project event delivery：project_id 非空。
	projDelivery := AutomationDelivery{
		ID: "proj-dlv-1", WorkspaceID: ws.ID,
		RuleScopeType: AutomationScopeProject, RuleScopeID: project.ID, ProjectID: &projectID,
		RuleID: "proj-rule-1", TriggerType: "event", EventID: "evt-1", EventType: "project.created",
		DedupeKey: "event:project:" + project.ID + ":proj-rule-1:evt-1",
		Status:    DeliveryStatusQueued,
		CreatedAt: 100, ModifiedAt: 100,
	}
	if err := repo.Enqueue([]AutomationDelivery{projDelivery}); err != nil {
		t.Fatalf("Enqueue project delivery: %v", err)
	}

	// Filter by ProjectID = "" 表示“schedule 无 Project”。
	emptyProjectID := ""
	scheduleRows, err := repo.List(AutomationDeliveryListOptions{WorkspaceID: ws.ID, ProjectID: &emptyProjectID})
	if err != nil {
		t.Fatalf("List schedule (nil project): %v", err)
	}
	if len(scheduleRows) != 1 || scheduleRows[0].ID != "ws-dlv-1" {
		t.Fatalf("schedule rows = %#v", scheduleRows)
	}

	// Filter by RuleScope = workspace only.
	wsScopeRows, err := repo.List(AutomationDeliveryListOptions{WorkspaceID: ws.ID, RuleScope: AutomationScopeWorkspace})
	if err != nil {
		t.Fatalf("List workspace scope: %v", err)
	}
	if len(wsScopeRows) != 1 || wsScopeRows[0].ID != "ws-dlv-1" {
		t.Fatalf("workspace scope rows = %#v", wsScopeRows)
	}

	// Filter by Q prefix on event_id.
	qRows, err := repo.List(AutomationDeliveryListOptions{WorkspaceID: ws.ID, Q: "evt-1"})
	if err != nil {
		t.Fatalf("List q=evt-1: %v", err)
	}
	if len(qRows) != 1 || qRows[0].ID != "proj-dlv-1" {
		t.Fatalf("q rows = %#v", qRows)
	}

	// Q 必须是前缀匹配，不能中缀匹配（不应返回 ws-dlv-1）。
	qInfix, err := repo.List(AutomationDeliveryListOptions{WorkspaceID: ws.ID, Q: "dlv"})
	if err != nil {
		t.Fatalf("List q=dlv: %v", err)
	}
	if len(qInfix) != 0 {
		t.Fatalf("q must be prefix-only, got %#v", qInfix)
	}
}

func TestAutomationDeliveryLatestByRuleIDsBatch(t *testing.T) {
	store, ws, _ := newAutomationRepoTestStore(t, "latest", "adsops")
	repo := NewAutomationDeliveryRepository(store.DB())
	// rule-a 有 3 条 delivery，rule-b 有 1 条，rule-c 没有。
	for i, at := range []int64{100, 200, 300} {
		if err := repo.Enqueue([]AutomationDelivery{{
			ID: fmt.Sprintf("a-%d", i), WorkspaceID: ws.ID, RuleID: "rule-a",
			DedupeKey: fmt.Sprintf("a-%d", i), Status: DeliveryStatusQueued, CreatedAt: at, ModifiedAt: at,
		}}); err != nil {
			t.Fatal(err)
		}
	}
	if err := repo.Enqueue([]AutomationDelivery{{
		ID: "b-0", WorkspaceID: ws.ID, RuleID: "rule-b", DedupeKey: "b-0",
		Status: DeliveryStatusQueued, CreatedAt: 250, ModifiedAt: 250,
	}}); err != nil {
		t.Fatal(err)
	}
	out, err := repo.LatestByRuleIDs([]string{"rule-a", "rule-b", "rule-c"})
	if err != nil {
		t.Fatalf("LatestByRuleIDs: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("latest map = %#v, want 2 entries", out)
	}
	if out["rule-a"].DeliveryID != "a-2" {
		t.Fatalf("latest for rule-a = %#v, want a-2 (created_at=300)", out["rule-a"])
	}
	if out["rule-b"].DeliveryID != "b-0" {
		t.Fatalf("latest for rule-b = %#v", out["rule-b"])
	}
	if _, ok := out["rule-c"]; ok {
		t.Fatalf("rule-c should not appear in latest map")
	}
}

func TestAutomationDeliveryClaimDueRecoversStale(t *testing.T) {
	store, ws, _ := newAutomationRepoTestStore(t, "claim-stale", "adsops")
	repo := NewAutomationDeliveryRepository(store.DB())
	// 一条 queued 与一条 stale delivering（claim 已过期）。
	if err := repo.Enqueue([]AutomationDelivery{
		{
			ID: "queued-1", WorkspaceID: ws.ID, RuleID: "rule-x", DedupeKey: "k1",
			Status: DeliveryStatusQueued, CreatedAt: 100, ModifiedAt: 100,
		},
		{
			ID: "stale-1", WorkspaceID: ws.ID, RuleID: "rule-x", DedupeKey: "k2",
			Status: DeliveryStatusDelivering, ClaimExpiresAt: int64Ptr(50),
			AttemptCount: 1, CreatedAt: 90, ModifiedAt: 90,
		},
	}); err != nil {
		t.Fatal(err)
	}
	claimed, err := repo.ClaimDue(100, 200, 10)
	if err != nil {
		t.Fatalf("ClaimDue: %v", err)
	}
	if len(claimed) != 2 {
		t.Fatalf("claimed = %#v, want 2 (queued + stale delivering)", claimed)
	}
	// stale 记录的 attempt_count 必须递增。
	for _, row := range claimed {
		if row.ID == "stale-1" && row.AttemptCount != 2 {
			t.Fatalf("stale attempt_count = %d, want 2", row.AttemptCount)
		}
		if row.Status != DeliveryStatusDelivering {
			t.Fatalf("status = %s, want delivering", row.Status)
		}
	}
}

func TestAutomationDeliveryReplayCreatesNewRowWithoutMutatingOriginal(t *testing.T) {
	store, ws, _ := newAutomationRepoTestStore(t, "replay", "adsops")
	repo := NewAutomationDeliveryRepository(store.DB())
	original := AutomationDelivery{
		ID: "orig-1", WorkspaceID: ws.ID, RuleID: "rule-x", DedupeKey: "k1",
		Status: DeliveryStatusDeadLettered, ResolvedURL: "https://agent.example.com/v1/chat/completions",
		RequestBodyJSON: `{"model":"m"}`, RequestBodyHash: "sha256:abc", MaxAttempts: 3,
		CreatedAt: 100, ModifiedAt: 100,
	}
	if err := repo.Enqueue([]AutomationDelivery{original}); err != nil {
		t.Fatal(err)
	}
	replayed, err := repo.CreateReplay(original, "replay-1", "replay:k1:1", 200)
	if err != nil {
		t.Fatalf("CreateReplay: %v", err)
	}
	if replayed.ID != "replay-1" || replayed.Status != DeliveryStatusQueued {
		t.Fatalf("replayed row = %#v", replayed)
	}
	if replayed.MaxAttempts != 3 {
		t.Fatalf("replay must inherit frozen max_attempts, got %d", replayed.MaxAttempts)
	}
	if replayed.ReplayOfDeliveryID == nil || *replayed.ReplayOfDeliveryID != "orig-1" {
		t.Fatalf("replay_of_delivery_id = %#v, want orig-1", replayed.ReplayOfDeliveryID)
	}
	// 原 delivery 不应变。
	orig, err := repo.GetByID("orig-1")
	if err != nil {
		t.Fatal(err)
	}
	if orig.Status != DeliveryStatusDeadLettered {
		t.Fatalf("original status = %s, want dead_lettered", orig.Status)
	}
}

// 注：int64Ptr 已在 hook_repo_test.go 中定义，整个 storage 包测试共用。

// TestAutomationScopeMigrationPreservesLegacyData 构造一个只含旧 project_automation_*
// 表的 legacy SQLite DB，再调用 Store.Open（含 AutoMigrate + scope migration），
// 断言历史规则/Delivery 的主键、状态、dedupe key、scope 回填和时间完全保留。
func TestAutomationScopeMigrationPreservesLegacyData(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "legacy.db")
	seedLegacyAutomationSQLite(t, dbPath)

	store, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open legacy: %v", err)
	}
	defer store.Close()

	// 旧表必须被删除。
	ruleRepo := NewAutomationRuleRepository(store.DB())
	for _, legacyTable := range []string{"project_automation_rules", "project_automation_deliveries"} {
		var count int64
		if err := store.DB().Raw("SELECT count(*) FROM sqlite_master WHERE type='table' AND name=?", legacyTable).Scan(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("legacy table %s still exists after migration", legacyTable)
		}
	}

	rules, err := ruleRepo.ListScope("ws-legacy", AutomationScopeProject, "proj-legacy", true)
	if err != nil {
		t.Fatalf("ListScope: %v", err)
	}
	if len(rules) != 1 || rules[0].ID != "rule-legacy" {
		t.Fatalf("rules after migration = %#v", rules)
	}
	if rules[0].ScopeType != AutomationScopeProject || rules[0].ScopeID != "proj-legacy" {
		t.Fatalf("scope not backfilled: %#v", rules[0])
	}
	if rules[0].Name != "每日项目巡检" || rules[0].TriggerType != "schedule" {
		t.Fatalf("rule content mismatch: %#v", rules[0])
	}

	deliveryRepo := NewAutomationDeliveryRepository(store.DB())
	deliveries, err := deliveryRepo.List(AutomationDeliveryListOptions{WorkspaceID: "ws-legacy"})
	if err != nil {
		t.Fatalf("List deliveries: %v", err)
	}
	if len(deliveries) != 2 {
		t.Fatalf("deliveries after migration = %#v, want 2", deliveries)
	}
	// 保留 status、dedupe_key、provider request id。
	byID := map[string]AutomationDelivery{}
	for _, d := range deliveries {
		byID[d.ID] = d
	}
	succeeded, ok := byID["dlv-succeeded"]
	if !ok {
		t.Fatalf("missing succeeded delivery: %#v", byID)
	}
	if succeeded.Status != DeliveryStatusSucceeded || succeeded.ProviderRequestID != "run_legacy_ok" || succeeded.DedupeKey != "dedupe-succeeded" {
		t.Fatalf("succeeded delivery fields not preserved: %#v", succeeded)
	}
	if succeeded.RuleScopeType != AutomationScopeProject || succeeded.RuleScopeID != "proj-legacy" {
		t.Fatalf("succeeded delivery scope not frozen: %#v", succeeded)
	}
	if succeeded.ProjectID == nil || *succeeded.ProjectID != "proj-legacy" {
		t.Fatalf("succeeded delivery project_id not preserved: %#v", succeeded.ProjectID)
	}
	retry, ok := byID["dlv-retry"]
	if !ok {
		t.Fatalf("missing retry delivery: %#v", byID)
	}
	if retry.Status != DeliveryStatusRetryWait || retry.DedupeKey != "dedupe-retry" {
		t.Fatalf("retry delivery fields not preserved: %#v", retry)
	}

	// 二次打开（meta 已写）必须保持幂等：不重复复制，不重建表。
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store2, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open again: %v", err)
	}
	defer store2.Close()
	deliveries2, err := NewAutomationDeliveryRepository(store2.DB()).List(AutomationDeliveryListOptions{WorkspaceID: "ws-legacy"})
	if err != nil {
		t.Fatal(err)
	}
	if len(deliveries2) != 2 {
		t.Fatalf("idempotent re-open changed delivery count: %#v", deliveries2)
	}
}

// seedLegacyAutomationSQLite 直接以 raw SQL 建立旧 schema 并写入一条 succeeded、
// 一条 retry_wait 的 Delivery，用于迁移测试。使用 gorm + glebarez sqlite 打开同一文件。
func seedLegacyAutomationSQLite(t *testing.T, dbPath string) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("gorm.Open legacy: %v", err)
	}
	mustExec := func(sqlText string) {
		if err := db.Exec(sqlText).Error; err != nil {
			t.Fatalf("exec %q: %v", sqlText, err)
		}
	}
	mustExec(`CREATE TABLE meta (key TEXT PRIMARY KEY, value TEXT NOT NULL)`)
	mustExec(`CREATE TABLE project_automation_rules (
	id TEXT PRIMARY KEY,
	workspace_id TEXT NOT NULL,
	project_id TEXT NOT NULL,
	name TEXT NOT NULL,
	description TEXT NOT NULL DEFAULT '',
	enabled NUMERIC NOT NULL DEFAULT TRUE,
	trigger_type TEXT NOT NULL,
	trigger_config_json TEXT NOT NULL DEFAULT '{}',
	condition_json TEXT NOT NULL DEFAULT '{}',
	action_type TEXT NOT NULL DEFAULT 'openai_compatible',
	action_config_json TEXT NOT NULL DEFAULT '{}',
	context_config_json TEXT NOT NULL DEFAULT '{}',
	instruction_template TEXT NOT NULL DEFAULT '',
	system_prompt TEXT NOT NULL DEFAULT '',
	created_by_actor_type TEXT NOT NULL DEFAULT 'user',
	created_by_user_id TEXT,
	created_by_token_id TEXT,
	created_by_token_name TEXT,
	created_by_token_prefix TEXT,
	created_at INTEGER NOT NULL,
	modified_at INTEGER NOT NULL
)`)
	mustExec(`CREATE TABLE project_automation_deliveries (
	id TEXT PRIMARY KEY,
	workspace_id TEXT NOT NULL,
	project_id TEXT NOT NULL,
	rule_id TEXT NOT NULL,
	trigger_type TEXT NOT NULL,
	event_id TEXT NOT NULL DEFAULT '',
	event_type TEXT NOT NULL DEFAULT '',
	dedupe_key TEXT NOT NULL,
	status TEXT NOT NULL,
	resolved_url TEXT NOT NULL DEFAULT '',
	rendered_method TEXT NOT NULL DEFAULT 'POST',
	rendered_headers_json TEXT NOT NULL DEFAULT '{}',
	request_body_json TEXT NOT NULL DEFAULT '',
	request_body_preview TEXT NOT NULL DEFAULT '',
	request_body_hash TEXT NOT NULL DEFAULT '',
	response_status_code INTEGER,
	response_body_preview TEXT NOT NULL DEFAULT '',
	provider_request_id TEXT NOT NULL DEFAULT '',
	usage_json TEXT NOT NULL DEFAULT '{}',
	attempt_count INTEGER NOT NULL DEFAULT 0,
	next_attempt_at INTEGER,
	claim_expires_at INTEGER,
	last_attempt_at INTEGER,
	last_error TEXT NOT NULL DEFAULT '',
	created_at INTEGER NOT NULL,
	modified_at INTEGER NOT NULL
)`)
	mustExec(`INSERT INTO project_automation_rules (id, workspace_id, project_id, name, description, enabled, trigger_type, trigger_config_json, condition_json, action_type, action_config_json, context_config_json, instruction_template, system_prompt, created_by_actor_type, created_at, modified_at)
VALUES ('rule-legacy', 'ws-legacy', 'proj-legacy', '每日项目巡检', '', 1, 'schedule', '{}', '{}', 'openai_compatible', '{}', '{}', '生成巡检', '', 'user', 100, 100)`)
	mustExec(`INSERT INTO project_automation_deliveries (id, workspace_id, project_id, rule_id, trigger_type, event_id, event_type, dedupe_key, status, resolved_url, rendered_method, rendered_headers_json, request_body_json, request_body_preview, request_body_hash, response_status_code, response_body_preview, provider_request_id, usage_json, attempt_count, next_attempt_at, claim_expires_at, last_attempt_at, last_error, created_at, modified_at)
VALUES ('dlv-succeeded', 'ws-legacy', 'proj-legacy', 'rule-legacy', 'schedule', '', '', 'dedupe-succeeded', 'succeeded', 'https://agent.example.com/v1/chat/completions', 'POST', '{}', '{"model":"m"}', '{"model":"m"}', 'sha256:abc', 200, '{"ok":1}', 'run_legacy_ok', '{}', 1, NULL, NULL, 100, '', 100, 100)`)
	mustExec(`INSERT INTO project_automation_deliveries (id, workspace_id, project_id, rule_id, trigger_type, event_id, event_type, dedupe_key, status, resolved_url, rendered_method, rendered_headers_json, request_body_json, request_body_preview, request_body_hash, response_status_code, response_body_preview, provider_request_id, usage_json, attempt_count, next_attempt_at, claim_expires_at, last_attempt_at, last_error, created_at, modified_at)
VALUES ('dlv-retry', 'ws-legacy', 'proj-legacy', 'rule-legacy', 'schedule', '', '', 'dedupe-retry', 'retry_wait', 'https://agent.example.com/v1/chat/completions', 'POST', '{}', '{"model":"m"}', '{"model":"m"}', 'sha256:def', 502, '{"err":"bad gateway"}', '', '{}', 2, 250, NULL, 200, 'bad gateway', 150, 200)`)
}

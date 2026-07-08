package storage

import (
	"path/filepath"
	"testing"
)

func newProjectAutomationRepoTest(t *testing.T, workspaceSlug string, projectSlug string) (*Store, Workspace, Project) {
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

func TestProjectAutomationRuleRepositoryLifecycle(t *testing.T) {
	store, ws, project := newProjectAutomationRepoTest(t, "auto", "adsops")
	repo := NewProjectAutomationRuleRepository(store.DB())
	enabled := true
	row := ProjectAutomationRule{
		ID:                "rule-1",
		WorkspaceID:       ws.ID,
		ProjectID:         project.ID,
		Name:              "每日项目巡检",
		Description:       "每天检查项目",
		Enabled:           &enabled,
		TriggerType:       "schedule",
		TriggerConfigJSON: `{"schedule_type":"daily_at","schedule_value":"09:30","timezone":"Asia/Shanghai"}`,
		ConditionJSON:     `{"task_filter":"status:pending","max_tasks":50}`,
		ActionType:        "openai_compatible",
		ActionConfigJSON:  `{"protocol":"chat_completions","base_url_config_key":"agent.provider.base_url","api_key_config_key":"agent.provider.api_key","model_config_key":"agent.provider.model","temperature":0.2}`,
		ContextConfigJSON: `{"include":["workspace","project","task_summary","matched_tasks","project_config"]}`,
		InstructionTemplate: "生成项目巡检报告",
		CreatedByActorType: "user",
		CreatedByUserID:    stringPtr("user-1"),
		CreatedAt:          100,
		ModifiedAt:         100,
	}
	if err := repo.Create(row); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := repo.GetByID(row.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Name != row.Name || got.ProjectID != project.ID || got.ActionType != "openai_compatible" {
		t.Fatalf("rule mismatch: %#v", got)
	}
	list, err := repo.List(ws.ID, &project.ID, false)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 || list[0].ID != row.ID {
		t.Fatalf("List = %#v", list)
	}
	disabled := false
	got.Enabled = &disabled
	got.ModifiedAt = 200
	if err := repo.Update(got); err != nil {
		t.Fatalf("Update: %v", err)
	}
	list, err = repo.List(ws.ID, &project.ID, false)
	if err != nil {
		t.Fatalf("List disabled: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("disabled rule should be hidden, got %#v", list)
	}
	list, err = repo.List(ws.ID, &project.ID, true)
	if err != nil {
		t.Fatalf("List include disabled: %v", err)
	}
	if len(list) != 1 || list[0].ModifiedAt != 200 {
		t.Fatalf("include disabled list = %#v", list)
	}
}

func TestProjectAutomationDeliveryRepositoryQueueAndState(t *testing.T) {
	store, ws, project := newProjectAutomationRepoTest(t, "auto-delivery", "adsops")
	repo := NewProjectAutomationDeliveryRepository(store.DB())
	row := ProjectAutomationDelivery{
		ID:                   "delivery-1",
		WorkspaceID:          ws.ID,
		ProjectID:            project.ID,
		RuleID:               "rule-1",
		TriggerType:          "schedule",
		DedupeKey:            ws.ID + ":" + project.ID + ":rule-1:2026-07-08:09:30",
		Status:               DeliveryStatusQueued,
		ResolvedURL:          "https://agent.example.com/v1/chat/completions",
		RenderedMethod:       "POST",
		RenderedHeadersJSON:  `{"Content-Type":["application/json"],"Authorization":["Bearer ****"]}`,
		RequestBodyJSON:      `{"model":"project-operator"}`,
		RequestBodyPreview:   `{"model":"project-operator"}`,
		RequestBodyHash:      "sha256:abc",
		ResponseBodyPreview:  "",
		CreatedAt:            100,
		ModifiedAt:           100,
	}
	if err := repo.Enqueue([]ProjectAutomationDelivery{row}); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	if err := repo.Enqueue([]ProjectAutomationDelivery{row}); err != nil {
		t.Fatalf("Enqueue duplicate: %v", err)
	}
	list, err := repo.List(ProjectAutomationDeliveryListOptions{WorkspaceID: ws.ID, ProjectID: &project.ID, Limit: 10})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("dedupe failed, got %d", len(list))
	}
	claimed, err := repo.ClaimDue(120, 240, 10)
	if err != nil {
		t.Fatalf("ClaimDue: %v", err)
	}
	if len(claimed) != 1 || claimed[0].AttemptCount != 1 || claimed[0].Status != DeliveryStatusDelivering {
		t.Fatalf("claimed = %#v", claimed)
	}
	if err := repo.MarkSucceeded(row.ID, 130, 200, "run_123", `{"id":"run_123"}`, `{"prompt_tokens":10}`); err != nil {
		t.Fatalf("MarkSucceeded: %v", err)
	}
	got, err := repo.GetByID(row.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Status != DeliveryStatusSucceeded || got.ProviderRequestID != "run_123" || got.ResponseStatusCode == nil || *got.ResponseStatusCode != 200 {
		t.Fatalf("succeeded row = %#v", got)
	}
}

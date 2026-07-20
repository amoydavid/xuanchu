package storage

import (
	"fmt"
	"path/filepath"
	"testing"
)

func TestAutomationTemplateCandidatePageFiltersCountsAndUsesStableTieBreak(t *testing.T) {
	store := openIdentityTestStore(t)
	ws, err := store.LocalWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	repo := NewProjectAutomationRuleRepository(store.DB())
	enabled := true
	for i := 0; i < 105; i++ {
		trigger := "schedule"
		if i%2 == 1 {
			trigger = "event"
		}
		if err := repo.Create(ProjectAutomationRule{
			ID: fmt.Sprintf("rule-%03d", i), WorkspaceID: ws.ID, ProjectID: "source", Name: fmt.Sprintf("发布 %03d", i),
			Enabled: &enabled, TriggerType: trigger, TriggerConfigJSON: `{}`, ConditionJSON: `{}`, ActionConfigJSON: `{}`, ContextConfigJSON: `{}`,
			CreatedByActorType: "user", CreatedAt: 100, ModifiedAt: 100,
		}); err != nil {
			t.Fatal(err)
		}
	}
	page, err := repo.ListCandidatePage(ProjectAutomationCandidateListOptions{
		WorkspaceID: ws.ID, ProjectID: "source", Q: "发布", Enabled: "enabled", TriggerType: "schedule",
	}, 25, 10)
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 53 || len(page.Items) != 25 || page.Items[0].ID != "rule-020" {
		t.Fatalf("page = %#v", page)
	}
}

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
		RenderedMethod:        "POST",
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

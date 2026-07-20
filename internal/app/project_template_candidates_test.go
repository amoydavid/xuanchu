package app

import (
	"fmt"
	"testing"

	"git.dajee.net/dajee/xuanchu/internal/storage"
	"git.dajee.net/dajee/xuanchu/internal/task"
	"git.dajee.net/dajee/xuanchu/internal/taskseries"
)

func projectTemplateCandidateFixture(t *testing.T) (*Service, ProjectView, ProjectView) {
	t.Helper()
	f := newProjectTemplateFixture(t)
	source, err := f.owner.AddProject(AddProjectInput{Slug: "candsrc", Name: "候选来源"})
	if err != nil {
		t.Fatal(err)
	}
	other, err := f.owner.AddProject(AddProjectInput{Slug: "candoth", Name: "其他项目"})
	if err != nil {
		t.Fatal(err)
	}
	return f.owner, source, other
}

func testCandidateSeries(project ProjectView, id, title string, seq int64) taskseries.Series {
	return taskseries.Series{
		ID: id, WorkspaceID: project.WorkspaceID, ProjectID: project.ID, Title: title,
		Status: taskseries.StatusActive, RecurrenceRule: "weekly", FirstDue: 1000,
		ProjectSeq: &seq, CreatedBy: "candidate-user", CreatedAt: 1, ModifiedAt: 1,
	}
}

func TestProjectTemplateCandidateListsUseSourceScopeAndBoundedPages(t *testing.T) {
	svc, source, other := projectTemplateCandidateFixture(t)
	rows := make([]storage.Task, 0, 122)
	for i := 0; i < 120; i++ {
		seq := int64(i + 1)
		projectID := source.ID
		rows = append(rows, storage.Task{UUID: fmt.Sprintf("candidate-task-%03d", i), WorkspaceID: source.WorkspaceID, Title: "上线准备", Status: "pending", Entry: 100, Modified: 100, ProjectID: &projectID, ProjectSeq: &seq})
	}
	otherID := other.ID
	rows = append(rows, storage.Task{UUID: "candidate-other-task", WorkspaceID: source.WorkspaceID, Title: "上线准备", Status: "pending", Entry: 100, Modified: 100, ProjectID: &otherID})
	if err := svc.store.DB().Create(&rows).Error; err != nil {
		t.Fatal(err)
	}

	tasks, err := svc.ListProjectTemplateTaskCandidates(TaskCandidateListInput{
		SourceProjectRef: source.Slug, Q: "上线", Status: "all", Query: "project:candoth or status:pending", Sort: "entry", Limit: 50, Offset: 50,
	})
	if err != nil {
		t.Fatal(err)
	}
	if tasks.Total != 120 || len(tasks.Items) != 50 || tasks.Items[0].Ref != "candidate-task-050" {
		t.Fatalf("task page = %#v", tasks)
	}
	for _, item := range tasks.Items {
		if item.ProjectID != source.ID || item.SeriesID != nil || item.Status == "deleted" {
			t.Fatalf("task scope leaked: %#v", item)
		}
	}

	seriesRepo := storage.NewTaskSeriesRepository(svc.store.DB())
	series, err := seriesRepo.Create(testCandidateSeries(source, "candidate-series", "上线周报", 1))
	if err != nil {
		t.Fatal(err)
	}
	seriesPage, err := svc.ListProjectTemplateSeriesCandidates(SeriesCandidateListInput{SourceProjectRef: source.Slug, Q: "上线", Status: "all"})
	if err != nil || seriesPage.Total != 1 || len(seriesPage.Items) != 1 || seriesPage.Items[0].Ref != series.ID {
		t.Fatalf("series page = %#v, err=%v", seriesPage, err)
	}

	if err := storage.NewConfigDefinitionRepository(svc.store.DB()).Set(storage.ConfigDefinition{WorkspaceID: source.WorkspaceID, Key: "deploy.token", ValueType: "string", AllowedScopesJSON: `["project"]`, Label: "发布令牌", Secret: true, CreatedAt: 1, ModifiedAt: 1}); err != nil {
		t.Fatal(err)
	}
	if err := storage.NewConfigRepository(svc.store.DB()).Set(storage.ConfigKey{WorkspaceID: source.WorkspaceID, Scope: storage.ConfigScopeProject, ScopeID: source.ID, Key: "deploy.token"}, "secret"); err != nil {
		t.Fatal(err)
	}
	configs, err := svc.ListProjectTemplateConfigCandidates(ConfigCandidateListInput{SourceProjectRef: source.Slug, Q: "发布", Mode: "secret"})
	if err != nil || configs.Total != 1 || configs.Items[0].Ref != "deploy.token" || configs.Items[0].Mode != "secret" {
		t.Fatalf("config page = %#v, err=%v", configs, err)
	}

	enabled := true
	actorID := svc.Runtime().ActorUserID
	if err := storage.NewProjectAutomationRuleRepository(svc.store.DB()).Create(storage.ProjectAutomationRule{
		ID: "candidate-automation", WorkspaceID: source.WorkspaceID, ProjectID: source.ID, Name: "发布巡检", Enabled: &enabled,
		TriggerType: "schedule", TriggerConfigJSON: `{}`, ConditionJSON: `{}`, ActionConfigJSON: `{}`, ContextConfigJSON: `{}`,
		CreatedByActorType: "user", CreatedByUserID: &actorID, CreatedAt: 1, ModifiedAt: 1,
	}); err != nil {
		t.Fatal(err)
	}
	automations, err := svc.ListProjectTemplateAutomationCandidates(AutomationCandidateListInput{SourceProjectRef: source.Slug, Q: "发布", Status: "enabled", TriggerType: "schedule"})
	if err != nil || automations.Total != 1 || automations.Items[0].Ref != "candidate-automation" || automations.Items[0].CreatedBy.ID != actorID {
		t.Fatalf("automation page = %#v, err=%v", automations, err)
	}

	byRef, err := svc.ListProjectTemplateTaskCandidates(TaskCandidateListInput{
		SourceProjectRef: source.Slug,
		Refs:             []string{"candidate-task-119"},
		Status:           "completed",
		Q:                "does-not-match",
	})
	if err != nil || byRef.Total != 1 || len(byRef.Items) != 1 || byRef.Items[0].Ref != "candidate-task-119" {
		t.Fatalf("task ref page = %#v, err=%v", byRef, err)
	}
}

func TestProjectTemplateTaskCandidateUrgencyMatchesTaskListSemantics(t *testing.T) {
	svc, source, other := projectTemplateCandidateFixture(t)
	if err := svc.SetConfig("urgency.uda.estimate.coefficient", "20"); err != nil {
		t.Fatal(err)
	}
	repo := storage.NewTaskRepository(svc.store.DB())
	sourceSlug, sourceID := source.Slug, source.ID
	otherSlug, otherID := other.Slug, other.ID
	overdue := svc.clock.Unix()
	high := "H"
	for _, row := range []task.Task{
		{
			UUID: "candidate-urgency-proxy-first", WorkspaceID: source.WorkspaceID,
			Title: "代理排序靠前", Status: task.StatusPending, Entry: 1, Modified: 1,
			Project: &sourceSlug, ProjectID: &sourceID, Priority: &high, Due: &overdue,
		},
		{
			UUID: "candidate-urgency-proxy-tie", WorkspaceID: source.WorkspaceID,
			Title: "代理排序同分", Status: task.StatusPending, Entry: 1, Modified: 1,
			Project: &sourceSlug, ProjectID: &sourceID, Priority: &high, Due: &overdue,
		},
		{
			UUID: "candidate-urgency-real-first", WorkspaceID: source.WorkspaceID,
			Title: "完整 urgency 靠前", Status: task.StatusPending, Entry: 2, Modified: 2,
			Project: &sourceSlug, ProjectID: &sourceID, Tags: []string{"next"},
			UDAs: map[string]task.UDAValue{"estimate": {Name: "estimate", Raw: "3"}},
		},
		{
			UUID: "candidate-urgency-dependent", WorkspaceID: source.WorkspaceID,
			Title: "跨项目依赖者", Status: task.StatusPending, Entry: 3, Modified: 3,
			Project: &otherSlug, ProjectID: &otherID, Depends: []string{"candidate-urgency-real-first"},
		},
	} {
		if _, err := repo.Create(row); err != nil {
			t.Fatal(err)
		}
	}

	wantPage, err := svc.QueryTaskViews(TaskViewQuery{ProjectID: source.ID, Sort: "urgency"})
	if err != nil {
		t.Fatal(err)
	}
	if len(wantPage.Items) != 3 || wantPage.Items[0].ID != "candidate-urgency-real-first" ||
		wantPage.Items[1].ID != "candidate-urgency-proxy-first" || wantPage.Items[2].ID != "candidate-urgency-proxy-tie" {
		t.Fatalf("task list urgency order = %#v", wantPage.Items)
	}
	for offset, want := range []string{"candidate-urgency-real-first", "candidate-urgency-proxy-first", "candidate-urgency-proxy-tie"} {
		gotPage, err := svc.ListProjectTemplateTaskCandidates(TaskCandidateListInput{
			SourceProjectRef: source.Slug, Status: "all", Sort: "urgency", Limit: 1, Offset: offset,
		})
		if err != nil {
			t.Fatal(err)
		}
		if gotPage.Total != 3 || len(gotPage.Items) != 1 || gotPage.Items[0].Ref != want {
			t.Fatalf("candidate urgency page offset %d = %#v, want %q", offset, gotPage, want)
		}
	}
}

func TestProjectTemplateSeriesCandidateNextMatchesSeriesListSemantics(t *testing.T) {
	svc, source, _ := projectTemplateCandidateFixture(t)
	repo := storage.NewTaskSeriesRepository(svc.store.DB())
	first, err := repo.Create(taskseries.Series{
		ID: "candidate-series-first-due", WorkspaceID: source.WorkspaceID, ProjectID: source.ID,
		Title: "旧 first_due 靠前", Status: taskseries.StatusActive, RecurrenceRule: "weekly", FirstDue: 10,
		ProjectSeq: int64Ptr(1), CreatedBy: "candidate-user", CreatedAt: 1, ModifiedAt: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := repo.Create(taskseries.Series{
		ID: "candidate-series-next-first", WorkspaceID: source.WorkspaceID, ProjectID: source.ID,
		Title: "当前规则下一槽位靠前", Status: taskseries.StatusActive, RecurrenceRule: "weekly", FirstDue: 20,
		ProjectSeq: int64Ptr(2), CreatedBy: "candidate-user", CreatedAt: 2, ModifiedAt: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.AppendRuleVersion(second.ID, taskseries.RuleVersion{
		ID: "candidate-series-next-rule", SeriesID: second.ID, EffectiveFrom: 90,
		RecurrenceRule: "daily", CreatedBy: "candidate-user", CreatedAt: 3,
	}); err != nil {
		t.Fatal(err)
	}
	tie, err := repo.Create(taskseries.Series{
		ID: "candidate-series-next-tie", WorkspaceID: source.WorkspaceID, ProjectID: source.ID,
		Title: "相同下一槽位", Status: taskseries.StatusActive, RecurrenceRule: "weekly", FirstDue: 30,
		ProjectSeq: int64Ptr(3), CreatedBy: "candidate-user", CreatedAt: 3, ModifiedAt: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.AppendRuleVersion(tie.ID, taskseries.RuleVersion{
		ID: "candidate-series-tie-rule", SeriesID: tie.ID, EffectiveFrom: 90,
		RecurrenceRule: "daily", CreatedBy: "candidate-user", CreatedAt: 4,
	}); err != nil {
		t.Fatal(err)
	}

	wantPage, err := svc.ListTaskSeries(TaskSeriesListInput{ProjectID: source.ID, Status: "active", Sort: "next"})
	if err != nil {
		t.Fatal(err)
	}
	if len(wantPage.Items) != 3 || wantPage.Items[0].ID != second.ID || wantPage.Items[1].ID != tie.ID || wantPage.Items[2].ID != first.ID {
		t.Fatalf("series list next order = %#v", wantPage.Items)
	}
	for offset, want := range []string{second.ID, tie.ID, first.ID} {
		gotPage, err := svc.ListProjectTemplateSeriesCandidates(SeriesCandidateListInput{
			SourceProjectRef: source.Slug, Status: "active", Sort: "next", Limit: 1, Offset: offset,
		})
		if err != nil {
			t.Fatal(err)
		}
		if gotPage.Total != 3 || len(gotPage.Items) != 1 || gotPage.Items[0].Ref != want {
			t.Fatalf("candidate next page offset %d = %#v, want %q", offset, gotPage, want)
		}
	}
}

func TestCandidateSelectionReturnsCanonicalExplicitRefsHashAndRejectsLimit(t *testing.T) {
	svc, source, _ := projectTemplateCandidateFixture(t)
	defs := storage.NewConfigDefinitionRepository(svc.store.DB())
	configs := storage.NewConfigRepository(svc.store.DB())
	for _, key := range []string{"z.key", "a.key"} {
		if err := defs.Set(storage.ConfigDefinition{WorkspaceID: source.WorkspaceID, Key: key, ValueType: "string", AllowedScopesJSON: `["project"]`, Label: key, CreatedAt: 1, ModifiedAt: 1}); err != nil {
			t.Fatal(err)
		}
		if err := configs.Set(storage.ConfigKey{WorkspaceID: source.WorkspaceID, Scope: storage.ConfigScopeProject, ScopeID: source.ID, Key: key}, key); err != nil {
			t.Fatal(err)
		}
	}
	query := CandidateSelectionQuery{SourceProjectRef: source.Slug, Kind: "config", Config: &ConfigCandidateListInput{Mode: "all"}}
	first, err := svc.ResolveProjectTemplateCandidateSelection(query)
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.ResolveProjectTemplateCandidateSelection(query)
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(first.Refs) != "[a.key z.key]" || first.Total != 2 || first.SourceHash == "" || first.SourceHash != second.SourceHash {
		t.Fatalf("selection = %#v, second=%#v", first, second)
	}

	enabled := true
	rules := make([]storage.ProjectAutomationRule, 0, 201)
	for i := 0; i < 201; i++ {
		rules = append(rules, storage.ProjectAutomationRule{
			ID: fmt.Sprintf("selection-rule-%03d", i), WorkspaceID: source.WorkspaceID, ProjectID: source.ID, Name: fmt.Sprintf("rule-%03d", i), Enabled: &enabled,
			TriggerType: "event", TriggerConfigJSON: `{}`, ConditionJSON: `{}`, ActionConfigJSON: `{}`, ContextConfigJSON: `{}`, CreatedByActorType: "user", CreatedAt: int64(i), ModifiedAt: int64(i),
		})
	}
	if err := svc.store.DB().Create(&rules).Error; err != nil {
		t.Fatal(err)
	}
	_, err = svc.ResolveProjectTemplateCandidateSelection(CandidateSelectionQuery{SourceProjectRef: source.Slug, Kind: "automation", Automation: &AutomationCandidateListInput{Status: "all", TriggerType: "all"}})
	if runtimeCode(err) != "project_template_candidate_limit_exceeded" {
		t.Fatalf("limit error = %v", err)
	}
}

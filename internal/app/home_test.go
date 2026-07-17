package app

import (
	"testing"
	"time"

	"git.dajee.net/dajee/xuanchu/internal/storage"
)

func TestHomeBuildsPersonalWorkAndProjectAttention(t *testing.T) {
	store := newTestStore(t)
	loc := time.UTC
	now := time.Date(2026, 7, 17, 12, 0, 0, 0, loc).Unix()
	svc, err := NewService(ServiceOptions{Store: store, Clock: FixedClock{NowUnix: now, Loc: loc}})
	if err != nil {
		t.Fatal(err)
	}
	risk, err := svc.AddProject(AddProjectInput{Slug: "risk", Name: "Risk"})
	if err != nil {
		t.Fatal(err)
	}
	safe, err := svc.AddProject(AddProjectInput{Slug: "safe", Name: "Safe"})
	if err != nil {
		t.Fatal(err)
	}
	// safe 最近有修改，但 risk 有真实风险，首页必须先展示 risk。
	if err := store.DB().Model(&storage.Project{}).Where("id = ?", safe.ID).Update("modified_at", now+100).Error; err != nil {
		t.Fatal(err)
	}
	overdueAt := time.Date(2026, 7, 16, 23, 59, 59, 0, loc).Unix()
	todayAt := time.Date(2026, 7, 17, 23, 59, 59, 0, loc).Unix()
	tomorrowAt := time.Date(2026, 7, 18, 23, 59, 59, 0, loc).Unix()
	high := "H"
	overdue, err := svc.AddTaskView(AddInput{Title: "overdue", Project: &risk.Slug, Due: &overdueAt, Assignees: []string{"local"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddTaskView(AddInput{Title: "today", Project: &risk.Slug, Due: &todayAt, Assignees: []string{"local"}}); err != nil {
		t.Fatal(err)
	}
	started, err := svc.AddTaskView(AddInput{Title: "started high", Project: &risk.Slug, Due: &tomorrowAt, Priority: &high, Assignees: []string{"local"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Start(started.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddTaskView(AddInput{Title: "unassigned safe", Project: &safe.Slug}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ProjectAnnotate(risk.Slug, "risk update"); err != nil {
		t.Fatal(err)
	}

	view, err := svc.Home()
	if err != nil {
		t.Fatal(err)
	}
	if view.Today != "2026-07-17" {
		t.Fatalf("today = %q", view.Today)
	}
	if view.MyWork == nil {
		t.Fatal("my_work = nil")
	}
	if view.MyWork.OpenCount != 3 || view.MyWork.StartedCount != 1 || view.MyWork.OverdueCount != 1 || view.MyWork.DueTodayCount != 1 || view.MyWork.HighPriorityOpenCount != 1 {
		t.Fatalf("my_work counts = %#v", view.MyWork)
	}
	if len(view.MyWork.Items) != 3 || view.MyWork.Items[0].Task.ID != started.ID {
		t.Fatalf("items = %#v, want started first", view.MyWork.Items)
	}
	if len(view.MyWork.Items[0].Reasons) == 0 || view.MyWork.Items[0].Reasons[0] != HomeTaskReasonStarted {
		t.Fatalf("started reasons = %#v", view.MyWork.Items[0].Reasons)
	}
	if view.MyWork.Items[1].Task.ID != overdue.ID {
		t.Fatalf("second item = %#v, want overdue", view.MyWork.Items[1])
	}
	if len(view.ProjectAttention) != 2 || view.ProjectAttention[0].Project.Slug != "risk" {
		t.Fatalf("project_attention = %#v", view.ProjectAttention)
	}
	if view.ProjectAttention[0].OverdueCount != 1 || view.ProjectAttention[0].LatestUpdate == nil || view.ProjectAttention[0].LatestUpdate.Content != "risk update" {
		t.Fatalf("risk attention = %#v", view.ProjectAttention[0])
	}
}

func TestHomeTenantActorOmitsPersonalWork(t *testing.T) {
	store := newTestStore(t)
	workspace, err := store.LocalWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	runtime := RuntimeContext{
		ActorType: "tenant_access_token", ActorTokenType: "tenant_access_token",
		ActorName: "system", WorkspaceID: workspace.ID, WorkspaceSlug: workspace.Slug, Role: RoleOwner,
	}
	scope := RequestScope{Capabilities: []string{"project:read"}, WorkspaceIDs: []string{workspace.ID}}
	svc, err := NewService(ServiceOptions{Store: store, Clock: FixedClock{NowUnix: 100, Loc: time.UTC}, Runtime: &runtime, RequestScope: &scope})
	if err != nil {
		t.Fatal(err)
	}
	view, err := svc.Home()
	if err != nil {
		t.Fatal(err)
	}
	if view.MyWork != nil {
		t.Fatalf("my_work = %#v, want nil", view.MyWork)
	}
}

func TestHomeRespectsProjectScope(t *testing.T) {
	store := newTestStore(t)
	ownerSvc := newTestServiceWithRuntime(t, store, 100, "local", "local")
	alpha, err := ownerSvc.AddProject(AddProjectInput{Slug: "alpha", Name: "Alpha"})
	if err != nil {
		t.Fatal(err)
	}
	beta, err := ownerSvc.AddProject(AddProjectInput{Slug: "beta", Name: "Beta"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ownerSvc.AddTaskView(AddInput{Title: "alpha task", Project: &alpha.Slug, Assignees: []string{"local"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := ownerSvc.AddTaskView(AddInput{Title: "beta task", Project: &beta.Slug, Assignees: []string{"local"}}); err != nil {
		t.Fatal(err)
	}

	runtime := ownerSvc.Runtime()
	scope := RequestScope{
		Capabilities: []string{"task:read", "project:read"},
		WorkspaceIDs: []string{runtime.WorkspaceID},
		ProjectIDs:   []string{alpha.ID},
	}
	scopedSvc, err := NewService(ServiceOptions{
		Store: store, Clock: FixedClock{NowUnix: 100, Loc: time.UTC}, Runtime: &runtime, RequestScope: &scope,
	})
	if err != nil {
		t.Fatal(err)
	}
	view, err := scopedSvc.Home()
	if err != nil {
		t.Fatal(err)
	}
	if view.MyWork == nil || len(view.MyWork.Items) != 1 || view.MyWork.Items[0].Task.ProjectID == nil || *view.MyWork.Items[0].Task.ProjectID != alpha.ID {
		t.Fatalf("scoped my_work = %#v", view.MyWork)
	}
	if len(view.ProjectAttention) != 1 || view.ProjectAttention[0].Project.ID != alpha.ID {
		t.Fatalf("scoped project_attention = %#v", view.ProjectAttention)
	}
}

func TestHomeUsesCompleteUserInfo(t *testing.T) {
	store := newTestStore(t)
	svc := newTestServiceWithRuntime(t, store, 100, "local", "local")
	actorID := svc.Runtime().ActorUserID
	email := "alice@example.com"
	if err := store.DB().Model(&storage.User{}).Where("id = ?", actorID).Updates(map[string]any{
		"display_name": "Alice", "email": email,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := svc.BindExternalID(actorID, "feishu", "open_id", "ou_home_actor"); err != nil {
		t.Fatal(err)
	}
	project, err := svc.AddProject(AddProjectInput{Slug: "people", Name: "People"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddTaskView(AddInput{Title: "owned task", Project: &project.Slug, Assignees: []string{"local"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ProjectAnnotate(project.Slug, "actor update"); err != nil {
		t.Fatal(err)
	}

	view, err := svc.Home()
	if err != nil {
		t.Fatal(err)
	}
	user := view.MyWork.Items[0].Task.Assignees[0]
	if user.ID != actorID || user.Name != "local" || user.DisplayName != "Alice" || user.Email == nil || *user.Email != email {
		t.Fatalf("assignee = %#v", user)
	}
	if len(user.ExternalIDs) != 1 || user.ExternalIDs[0].Provider != "feishu" || user.ExternalIDs[0].UserType != "open_id" || user.ExternalIDs[0].ExternalID != "ou_home_actor" {
		t.Fatalf("assignee external IDs = %#v", user.ExternalIDs)
	}
	actor := view.ProjectAttention[0].LatestUpdate.CreatedBy
	if actor.Type != "user" || actor.User == nil || actor.User.ID != actorID || actor.User.DisplayName != "Alice" || len(actor.User.ExternalIDs) != 1 {
		t.Fatalf("latest update actor = %#v", actor)
	}
}

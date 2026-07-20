package app

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"git.dajee.net/dajee/xuanchu/internal/auth"
	"git.dajee.net/dajee/xuanchu/internal/authz"
	"git.dajee.net/dajee/xuanchu/internal/projecttemplate"
	"git.dajee.net/dajee/xuanchu/internal/storage"
)

type projectTemplateFixture struct {
	store *storage.Store
	owner *Service
}

func newProjectTemplateFixture(t *testing.T) projectTemplateFixture {
	t.Helper()
	store := newTestStore(t)
	owner, err := NewService(ServiceOptions{Store: store, Clock: FixedClock{NowUnix: 100}})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return projectTemplateFixture{store: store, owner: owner}
}

func projectTemplateSnapshotFixture() projecttemplate.SnapshotV1 {
	configValue := "https://api.example.test/v1"
	return projecttemplate.SnapshotV1{
		Schema:     projecttemplate.SnapshotSchemaV1,
		AnchorDate: "2026-07-20",
		Project:    projecttemplate.ProjectBlueprintV1{Description: "项目说明"},
		Configs: []projecttemplate.ConfigBlueprintV1{
			{Key: "agent.base_url", Mode: "literal", Value: &configValue},
			{Key: "agent.api_key", Mode: "secret_input"},
		},
		Tasks:  []projecttemplate.TaskBlueprintV1{{Ref: "task-1", Title: "准备发布"}},
		Series: []projecttemplate.SeriesBlueprintV1{},
		Automations: []projecttemplate.AutomationBlueprintV1{{
			Ref: "automation-1", Name: "发布巡检", TriggerType: "schedule",
			Action:  projecttemplate.AutomationActionV1{Protocol: "chat_completions", BaseURLConfigKey: "agent.base_url", APIKeyConfigKey: "agent.api_key", ModelConfigKey: "agent.model"},
			Context: projecttemplate.AutomationContextV1{Include: []string{"project"}}, InstructionTemplate: "检查发布状态",
		}},
	}
}

func seedProjectTemplate(t *testing.T, svc *Service, key string, snapshot projecttemplate.SnapshotV1) storage.ProjectTemplate {
	t.Helper()
	raw, hash, err := projecttemplate.EncodeV1(snapshot, projecttemplate.DefaultLimits)
	if err != nil {
		t.Fatalf("EncodeV1: %v", err)
	}
	now := svc.Clock().Unix()
	userID := svc.Runtime().ActorUserID
	template := storage.ProjectTemplate{
		ID: uuid.NewString(), WorkspaceID: svc.Runtime().WorkspaceID, Key: key, Name: "发布模板", Description: "发布流程", Status: "active",
		CreatedByActorType: actorTypeUser, CreatedByUserID: &userID, CreatedAt: now, ModifiedAt: now,
	}
	repo := storage.NewProjectTemplateRepository(svc.store.DB())
	if err := repo.Create(template); err != nil {
		t.Fatalf("create template: %v", err)
	}
	source, err := svc.ResolveProject("src")
	sourceID := source.ID
	if err != nil {
		created, err := svc.AddProject(AddProjectInput{Slug: "src", Name: "模板来源"})
		if err != nil {
			t.Fatalf("create source project: %v", err)
		}
		sourceID = created.ID
	}
	if err := svc.store.Transaction(func(txStore *storage.Store) error {
		_, err := storage.NewProjectTemplateRepository(txStore.DB()).AppendSnapshotLocked(template.WorkspaceID, template.ID, storage.ProjectTemplateSnapshot{
			ID: uuid.NewString(), SourceProjectID: sourceID, SnapshotJSON: string(raw), SnapshotHash: hash,
			CreatedByActorType: actorTypeUser, CreatedByUserID: &userID, CreatedAt: now,
		})
		return err
	}); err != nil {
		t.Fatalf("append snapshot: %v", err)
	}
	row, err := repo.GetByRef(template.WorkspaceID, template.ID)
	if err != nil {
		t.Fatalf("get template: %v", err)
	}
	return row
}

func runtimeCode(err error) string {
	var runtimeErr RuntimeError
	if errors.As(err, &runtimeErr) {
		return runtimeErr.Code
	}
	var permissionErr PermissionError
	if errors.As(err, &permissionErr) {
		return permissionErr.Code
	}
	return ""
}

func TestProjectTemplateMetadataLifecycleAndRedaction(t *testing.T) {
	f := newProjectTemplateFixture(t)
	seeded := seedProjectTemplate(t, f.owner, "launch", projectTemplateSnapshotFixture())

	page, err := f.owner.ListProjectTemplates("all", "launch", 50, 0)
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("ListProjectTemplates = %#v, %v", page, err)
	}
	raw, err := json.Marshal(page)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "snapshot_json") {
		t.Fatalf("metadata leaked raw snapshot: %s", raw)
	}
	if page.Items[0].CreatedBy.User == nil || page.Items[0].CreatedBy.User.ID != f.owner.Runtime().ActorUserID {
		t.Fatalf("created_by = %#v", page.Items[0].CreatedBy)
	}

	archived, err := f.owner.ArchiveProjectTemplate(seeded.Key)
	if err != nil || archived.Template.Status != "archived" || archived.Template.ArchivedAt == nil {
		t.Fatalf("ArchiveProjectTemplate = %#v, %v", archived, err)
	}
	reactivated, err := f.owner.ReactivateProjectTemplate(seeded.ID)
	if err != nil || reactivated.Template.Status != "active" || reactivated.Template.ArchivedAt != nil {
		t.Fatalf("ReactivateProjectTemplate = %#v, %v", reactivated, err)
	}
}

func TestProjectTemplatePermissionScopeAndInstantiationList(t *testing.T) {
	f := newProjectTemplateFixture(t)
	seeded := seedProjectTemplate(t, f.owner, "launch", projectTemplateSnapshotFixture())

	viewerRuntime := f.owner.Runtime()
	viewerRuntime.Role = RoleViewer
	viewer, err := NewService(ServiceOptions{Store: f.store, Clock: FixedClock{NowUnix: 100}, Runtime: &viewerRuntime, DisableScopeBootstrap: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := viewer.ListProjectTemplates("active", "", 50, 0); err != nil {
		t.Fatalf("viewer list: %v", err)
	}
	if _, err := viewer.ArchiveProjectTemplate(seeded.Key); runtimeCode(err) != authz.CodePermissionDenied {
		t.Fatalf("viewer archive error = %v", err)
	}
	if _, err := viewer.ProjectTemplateInfo(seeded.Key, nil); runtimeCode(err) != authz.CodePermissionDenied {
		t.Fatalf("viewer detail error = %v", err)
	}

	instantiation, err := f.owner.ListProjectTemplatesForInstantiation(TemplateInstantiationListInput{Limit: 50})
	if err != nil || len(instantiation.Items) != 1 {
		t.Fatalf("ListProjectTemplatesForInstantiation = %#v, %v", instantiation, err)
	}
	current := instantiation.Items[0].CurrentSnapshot
	if current == nil || current.ID == "" || current.Version != 1 || current.Hash == "" || len(current.RequiredSecretKeys) != 1 || current.RequiredSecretKeys[0] != "agent.api_key" {
		t.Fatalf("instantiation current snapshot = %#v", current)
	}
	if strings.Contains(string(mustJSON(instantiation)), "snapshot_json") {
		t.Fatalf("instantiation list leaked snapshot: %s", mustJSON(instantiation))
	}

	projectScope := &RequestScope{WorkspaceIDs: []string{f.owner.Runtime().WorkspaceID}, ProjectIDs: []string{"only-project"}, Capabilities: []string{auth.ScopeProjectRead}}
	scopedRuntime := f.owner.Runtime()
	scoped, err := NewService(ServiceOptions{Store: f.store, Clock: FixedClock{NowUnix: 100}, Runtime: &scopedRuntime, RequestScope: projectScope, DisableScopeBootstrap: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := scoped.ListProjectTemplates("active", "", 50, 0); runtimeCode(err) != authz.CodeProjectScopeDenied {
		t.Fatalf("project-scoped list error = %v", err)
	}
}

func TestProjectTemplateModifyAndLifecycleAudit(t *testing.T) {
	f := newProjectTemplateFixture(t)
	seeded := seedProjectTemplate(t, f.owner, "launch", projectTemplateSnapshotFixture())
	name, description := "新版发布模板", "新的说明"
	modified, err := f.owner.ModifyProjectTemplate(seeded.Key, ModifyTemplateInput{Name: &name, Description: &description})
	if err != nil || modified.Template.Name != name || modified.Template.Description != description {
		t.Fatalf("ModifyProjectTemplate = %#v, %v", modified, err)
	}
	if _, err := f.owner.ArchiveProjectTemplate(seeded.Key); err != nil {
		t.Fatal(err)
	}
	if _, err := f.owner.ReactivateProjectTemplate(seeded.Key); err != nil {
		t.Fatal(err)
	}

	action := "project_template.modify"
	rows, err := storage.NewAuditRepository(f.store.DB()).List(storage.AuditListOptions{WorkspaceID: &seeded.WorkspaceID, TargetID: &seeded.ID, Action: &action, Limit: 10})
	if err != nil || len(rows) < 2 {
		t.Fatalf("modify audit rows = %#v, %v", rows, err)
	}
	var foundReactivate bool
	for _, row := range rows {
		if strings.Contains(row.PayloadJSON, `"status_before":"archived"`) && strings.Contains(row.PayloadJSON, `"status_after":"active"`) {
			foundReactivate = true
		}
		if strings.Contains(row.PayloadJSON, "snapshot_json") || strings.Contains(row.PayloadJSON, "agent.api_key") {
			t.Fatalf("audit leaked snapshot or secret: %s", row.PayloadJSON)
		}
	}
	if !foundReactivate {
		t.Fatalf("reactivate must use project_template.modify with status payload: %#v", rows)
	}
}

func TestProjectTemplateDetailRequiresComponentPermissionsAndStaysInWorkspace(t *testing.T) {
	f := newProjectTemplateFixture(t)
	seeded := seedProjectTemplate(t, f.owner, "launch", projectTemplateSnapshotFixture())

	tenantRead := newScopedTokenService(t, f.store, []string{auth.ScopeProjectRead}, auth.TokenTypeTenantAccess)
	if _, err := tenantRead.ListProjectTemplates("active", "", 50, 0); err != nil {
		t.Fatalf("tenant metadata list: %v", err)
	}
	if _, err := tenantRead.ProjectTemplateInfo(seeded.Key, nil); runtimeCode(err) != authz.CodePermissionDenied {
		t.Fatalf("tenant detail without task/config/hook read = %v", err)
	}
	tenantDetail := newScopedTokenService(t, f.store, []string{auth.ScopeProjectRead, auth.ScopeTaskRead, auth.ScopeConfigRead, auth.ScopeHookRead}, auth.TokenTypeTenantAccess)
	detail, err := tenantDetail.ProjectTemplateInfo(seeded.ID, nil)
	if err != nil || detail.Snapshot == nil || len(detail.Snapshot.Configs) != 2 {
		t.Fatalf("tenant detail = %#v, %v", detail, err)
	}
	for _, config := range detail.Snapshot.Configs {
		if config.Key == "agent.api_key" && config.Value != nil {
			t.Fatalf("secret config value leaked: %#v", config)
		}
	}
	if strings.Contains(mustJSON(detail), "snapshot_json") {
		t.Fatalf("detail leaked raw snapshot: %s", mustJSON(detail))
	}

	otherWorkspace := mustCreateWorkspaceRecord(t, f.store, storage.Workspace{ID: "ws-other", Slug: "other", Name: "Other", CreatedAt: 100, ModifiedAt: 100})
	mustUpsertMembershipRecord(t, f.store, storage.Membership{UserID: f.owner.Runtime().ActorUserID, WorkspaceID: otherWorkspace.ID, Role: string(RoleOwner), JoinedAt: 100, ModifiedAt: 100})
	other := newTestServiceWithRuntime(t, f.store, 100, "local", otherWorkspace.Slug)
	otherTemplate := seedProjectTemplate(t, other, "other-launch", projectTemplateSnapshotFixture())
	if _, err := f.owner.ProjectTemplateInfo(otherTemplate.ID, nil); runtimeCode(err) != "project_template_not_found" {
		t.Fatalf("cross-workspace detail error = %v", err)
	}
}

func TestProjectTemplateListUsesDatabaseOrderedPages(t *testing.T) {
	f := newProjectTemplateFixture(t)
	first := seedProjectTemplate(t, f.owner, "launch", projectTemplateSnapshotFixture())
	second := seedProjectTemplate(t, f.owner, "later", projectTemplateSnapshotFixture())
	if err := storage.NewProjectTemplateRepository(f.store.DB()).UpdateMetadata(second.WorkspaceID, second.ID, second.Name, second.Description, 200); err != nil {
		t.Fatal(err)
	}

	page, err := f.owner.ListProjectTemplates("all", "", 1, 0)
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != second.ID || page.Total != 2 {
		t.Fatalf("first ordered page = %#v, %v", page, err)
	}
	next, err := f.owner.ListProjectTemplates("all", "", 1, 1)
	if err != nil || len(next.Items) != 1 || next.Items[0].ID != first.ID {
		t.Fatalf("second ordered page = %#v, %v", next, err)
	}
}

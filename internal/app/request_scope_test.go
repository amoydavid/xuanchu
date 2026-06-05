package app

import (
	"testing"

	"git.dajee.net/dajee/xuanchu/internal/storage"
	"git.dajee.net/dajee/xuanchu/internal/task"
)

func TestAuthorizeTokenRequestRejectsMissingCapability(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()

	created, err := svc.CreateToken(CreateTokenInput{
		Name:          "cli",
		Scopes:        []string{"project:read"},
		WorkspaceRefs: []string{"local"},
	})
	if err != nil {
		t.Fatal(err)
	}
	authn, err := svc.AuthenticateBearerToken(created.RawToken)
	if err != nil {
		t.Fatal(err)
	}

	_, err = svc.AuthorizeTokenRequest(RequestAuthorizationInput{
		Token:              authn,
		RequiredCapability: "task:read",
		RequiredPermission: PermissionTaskRead,
		WorkspaceRef:       "local",
	})
	assertRuntimeCode(t, err, "token_scope_denied")
}

func TestProjectScopedServiceFiltersReadAndWrite(t *testing.T) {
	store := newTestStore(t)
	ownerSvc := newTestServiceWithRuntime(t, store, 100, "local", "local")

	work, err := ownerSvc.AddProject(AddProjectInput{Slug: "work", Name: "Work"})
	if err != nil {
		t.Fatalf("AddProject(work) error = %v", err)
	}
	home, err := ownerSvc.AddProject(AddProjectInput{Slug: "home", Name: "Home"})
	if err != nil {
		t.Fatalf("AddProject(home) error = %v", err)
	}
	workTask, err := ownerSvc.Add(AddInput{Description: "work task", Project: strptr(work.Slug)})
	if err != nil {
		t.Fatalf("Add(work task) error = %v", err)
	}
	homeTask, err := ownerSvc.Add(AddInput{Description: "home task", Project: strptr(home.Slug)})
	if err != nil {
		t.Fatalf("Add(home task) error = %v", err)
	}
	if _, err := ownerSvc.Add(AddInput{Description: "inbox task"}); err != nil {
		t.Fatalf("Add(inbox task) error = %v", err)
	}

	created, err := ownerSvc.CreateToken(CreateTokenInput{
		Name:          "cli",
		Scopes:        []string{"task:read", "task:write", "project:read"},
		WorkspaceRefs: []string{"local"},
		ProjectRefs:   []string{work.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	authn, err := ownerSvc.AuthenticateBearerToken(created.RawToken)
	if err != nil {
		t.Fatal(err)
	}
	authorized, err := ownerSvc.AuthorizeTokenRequest(RequestAuthorizationInput{
		Token:              authn,
		RequiredCapability: "task:read",
		RequiredPermission: PermissionTaskRead,
		WorkspaceRef:       "local",
	})
	if err != nil {
		t.Fatalf("AuthorizeTokenRequest() error = %v", err)
	}
	scopedSvc, err := NewService(ServiceOptions{
		Store:        store,
		Clock:        FixedClock{NowUnix: 100},
		Runtime:      &authorized.Runtime,
		RequestScope: &authorized.Scope,
	})
	if err != nil {
		t.Fatalf("NewService(scoped) error = %v", err)
	}

	tasks, err := scopedSvc.List(ListInput{})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(tasks) != 1 || tasks[0].UUID != workTask.UUID {
		t.Fatalf("scoped list tasks = %#v, want only work task", tasks)
	}

	if _, err := scopedSvc.Info(homeTask.UUID); err == nil {
		t.Fatal("Info(home task) error = nil")
	} else {
		assertRuntimeCode(t, err, "task_not_found")
	}

	if err := scopedSvc.Modify(homeTask.UUID, ModifyInput{Description: strptr("blocked")}); err == nil {
		t.Fatal("Modify(home task) error = nil")
	} else {
		assertRuntimeCode(t, err, "project_scope_denied")
	}

	if _, err := scopedSvc.Add(AddInput{Description: "scoped inbox"}); err == nil {
		t.Fatal("Add(no project) error = nil")
	} else {
		assertRuntimeCode(t, err, "project_scope_denied")
	}
}

func TestProjectScopedImportCannotClearProject(t *testing.T) {
	store := newTestStore(t)
	ownerSvc := newTestServiceWithRuntime(t, store, 100, "local", "local")

	work, err := ownerSvc.AddProject(AddProjectInput{Slug: "work", Name: "Work"})
	if err != nil {
		t.Fatalf("AddProject(work) error = %v", err)
	}
	workTask, err := ownerSvc.Add(AddInput{Description: "work task", Project: strptr(work.Slug)})
	if err != nil {
		t.Fatalf("Add(work task) error = %v", err)
	}
	created, err := ownerSvc.CreateToken(CreateTokenInput{
		Name:          "cli",
		Scopes:        []string{"task:write"},
		WorkspaceRefs: []string{"local"},
		ProjectRefs:   []string{work.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	authn, err := ownerSvc.AuthenticateBearerToken(created.RawToken)
	if err != nil {
		t.Fatal(err)
	}
	authorized, err := ownerSvc.AuthorizeTokenRequest(RequestAuthorizationInput{
		Token:              authn,
		RequiredCapability: "task:write",
		RequiredPermission: PermissionTaskWrite,
		WorkspaceRef:       "local",
	})
	if err != nil {
		t.Fatalf("AuthorizeTokenRequest() error = %v", err)
	}
	scopedSvc, err := NewService(ServiceOptions{
		Store:        store,
		Clock:        FixedClock{NowUnix: 100},
		Runtime:      &authorized.Runtime,
		RequestScope: &authorized.Scope,
	})
	if err != nil {
		t.Fatalf("NewService(scoped) error = %v", err)
	}

	payload := task.ToJSON(workTask)
	payload.Project = nil
	_, err = scopedSvc.Import([]task.JSONTask{payload})
	assertRuntimeCode(t, err, "project_scope_denied")

	reloaded, err := ownerSvc.Info(workTask.UUID)
	if err != nil {
		t.Fatalf("Info(workTask) error = %v", err)
	}
	if reloaded.ProjectID == nil || *reloaded.ProjectID != work.ID {
		t.Fatalf("project was cleared after failed import: %#v", reloaded.ProjectID)
	}
}

func TestProjectScopedServiceFiltersProjectsAndAudit(t *testing.T) {
	store := newTestStore(t)
	ownerSvc := newTestServiceWithRuntime(t, store, 100, "local", "local")

	alpha, err := ownerSvc.AddProject(AddProjectInput{Slug: "alpha", Name: "Alpha"})
	if err != nil {
		t.Fatalf("AddProject(alpha) error = %v", err)
	}
	beta, err := ownerSvc.AddProject(AddProjectInput{Slug: "beta", Name: "Beta"})
	if err != nil {
		t.Fatalf("AddProject(beta) error = %v", err)
	}
	if _, err := ownerSvc.Add(AddInput{Description: "alpha task", Project: strptr(alpha.Slug)}); err != nil {
		t.Fatalf("Add(alpha task) error = %v", err)
	}
	if _, err := ownerSvc.Add(AddInput{Description: "beta task", Project: strptr(beta.Slug)}); err != nil {
		t.Fatalf("Add(beta task) error = %v", err)
	}

	created, err := ownerSvc.CreateToken(CreateTokenInput{
		Name:          "auditor",
		Scopes:        []string{"task:read", "project:read", "audit:read"},
		WorkspaceRefs: []string{"local"},
		ProjectRefs:   []string{alpha.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	authn, err := ownerSvc.AuthenticateBearerToken(created.RawToken)
	if err != nil {
		t.Fatal(err)
	}
	authorized, err := ownerSvc.AuthorizeTokenRequest(RequestAuthorizationInput{
		Token:              authn,
		RequiredCapability: "audit:read",
		RequiredPermission: PermissionAuditRead,
		WorkspaceRef:       "local",
	})
	if err != nil {
		t.Fatalf("AuthorizeTokenRequest() error = %v", err)
	}
	scopedSvc, err := NewService(ServiceOptions{
		Store:        store,
		Clock:        FixedClock{NowUnix: 100},
		Runtime:      &authorized.Runtime,
		RequestScope: &authorized.Scope,
	})
	if err != nil {
		t.Fatalf("NewService(scoped) error = %v", err)
	}

	projects, err := scopedSvc.ListProjects(false)
	if err != nil {
		t.Fatalf("ListProjects() error = %v", err)
	}
	if len(projects) != 1 || projects[0].ID != alpha.ID {
		t.Fatalf("scoped projects = %#v, want only alpha", projects)
	}

	rows, err := scopedSvc.ListAudit(AuditListInput{Limit: 50})
	if err != nil {
		t.Fatalf("ListAudit() error = %v", err)
	}
	for _, row := range rows {
		if row.ProjectID != nil && *row.ProjectID != alpha.ID {
			t.Fatalf("audit row leaked out-of-scope project: %#v", row)
		}
	}

	if _, err := scopedSvc.ProjectInfo(beta.ID); err == nil {
		t.Fatal("ProjectInfo(beta) error = nil")
	} else {
		assertRuntimeCode(t, err, "project_scope_denied")
	}
}

func TestAuthorizeTokenRequestRejectsProjectOutsideAllowlist(t *testing.T) {
	store := newTestStore(t)
	ownerSvc := newTestServiceWithRuntime(t, store, 100, "local", "local")

	alpha, err := ownerSvc.AddProject(AddProjectInput{Slug: "alpha", Name: "Alpha"})
	if err != nil {
		t.Fatalf("AddProject(alpha) error = %v", err)
	}
	beta, err := ownerSvc.AddProject(AddProjectInput{Slug: "beta", Name: "Beta"})
	if err != nil {
		t.Fatalf("AddProject(beta) error = %v", err)
	}

	created, err := ownerSvc.CreateToken(CreateTokenInput{
		Name:          "cli",
		Scopes:        []string{"project:read"},
		WorkspaceRefs: []string{"local"},
		ProjectRefs:   []string{alpha.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	authn, err := ownerSvc.AuthenticateBearerToken(created.RawToken)
	if err != nil {
		t.Fatal(err)
	}

	_, err = ownerSvc.AuthorizeTokenRequest(RequestAuthorizationInput{
		Token:              authn,
		RequiredCapability: "project:read",
		RequiredPermission: PermissionProjectRead,
		WorkspaceRef:       "local",
		ProjectRef:         beta.ID,
	})
	assertRuntimeCode(t, err, "project_scope_denied")
}

func TestAuthorizeTokenRequestProjectIDSelectsOwningWorkspace(t *testing.T) {
	store := newTestStore(t)
	ownerSvc := newTestServiceWithRuntime(t, store, 100, "local", "local")
	work, err := ownerSvc.AddWorkspace(AddWorkspaceInput{Slug: "work", Name: "Work"})
	if err != nil {
		t.Fatalf("AddWorkspace(work) error = %v", err)
	}
	workSvc := newTestServiceWithRuntime(t, store, 100, "local", work.Slug)
	project, err := workSvc.AddProject(AddProjectInput{Slug: "api", Name: "API"})
	if err != nil {
		t.Fatalf("AddProject(api) error = %v", err)
	}
	created, err := ownerSvc.CreateToken(CreateTokenInput{
		Name:   "agent",
		Scopes: []string{"project:read"},
	})
	if err != nil {
		t.Fatal(err)
	}
	authn, err := ownerSvc.AuthenticateBearerToken(created.RawToken)
	if err != nil {
		t.Fatal(err)
	}

	authorized, err := ownerSvc.AuthorizeTokenRequest(RequestAuthorizationInput{
		Token:              authn,
		RequiredCapability: "project:read",
		RequiredPermission: PermissionProjectRead,
		ProjectRef:         project.ID,
		ProjectRefIsID:     true,
	})
	if err != nil {
		t.Fatalf("AuthorizeTokenRequest() error = %v", err)
	}
	if authorized.Workspace.ID != work.ID {
		t.Fatalf("workspace = %q, want %q", authorized.Workspace.ID, work.ID)
	}
	if authorized.Project == nil || authorized.Project.ID != project.ID {
		t.Fatalf("project = %#v, want %q", authorized.Project, project.ID)
	}
}

func TestExplicitProjectScopeFiltersSingleTaskOperations(t *testing.T) {
	store := newTestStore(t)
	ownerSvc := newTestServiceWithRuntime(t, store, 100, "local", "local")
	alpha, err := ownerSvc.AddProject(AddProjectInput{Slug: "alpha", Name: "Alpha"})
	if err != nil {
		t.Fatalf("AddProject(alpha) error = %v", err)
	}
	beta, err := ownerSvc.AddProject(AddProjectInput{Slug: "beta", Name: "Beta"})
	if err != nil {
		t.Fatalf("AddProject(beta) error = %v", err)
	}
	alphaTask, err := ownerSvc.Add(AddInput{Description: "alpha task", Project: strptr(alpha.Slug)})
	if err != nil {
		t.Fatalf("Add(alpha task) error = %v", err)
	}
	created, err := ownerSvc.CreateToken(CreateTokenInput{
		Name:          "agent",
		Scopes:        []string{"task:read", "task:write"},
		WorkspaceRefs: []string{"local"},
	})
	if err != nil {
		t.Fatal(err)
	}
	authn, err := ownerSvc.AuthenticateBearerToken(created.RawToken)
	if err != nil {
		t.Fatal(err)
	}
	authorized, err := ownerSvc.AuthorizeTokenRequest(RequestAuthorizationInput{
		Token:              authn,
		RequiredCapability: "task:read",
		RequiredPermission: PermissionTaskRead,
		WorkspaceRef:       "local",
		ProjectRef:         beta.ID,
		ProjectRefIsID:     true,
	})
	if err != nil {
		t.Fatalf("AuthorizeTokenRequest() error = %v", err)
	}
	scopedSvc, err := NewService(ServiceOptions{
		Store:        store,
		Clock:        FixedClock{NowUnix: 100},
		Runtime:      &authorized.Runtime,
		RequestScope: &authorized.Scope,
	})
	if err != nil {
		t.Fatalf("NewService(scoped) error = %v", err)
	}

	if _, err := scopedSvc.Info(alphaTask.UUID); err == nil {
		t.Fatal("Info(alpha task through beta request scope) error = nil")
	} else {
		assertRuntimeCode(t, err, "task_not_found")
	}
	if err := scopedSvc.Modify(alphaTask.UUID, ModifyInput{Description: strptr("blocked")}); err == nil {
		t.Fatal("Modify(alpha task through beta request scope) error = nil")
	} else {
		assertRuntimeCode(t, err, "project_scope_denied")
	}
}

func TestAuthorizeTokenRequestImpersonationUsesSubjectMembership(t *testing.T) {
	store := newTestStore(t)
	ownerSvc := newTestServiceWithRuntime(t, store, 100, "local", "local")
	aliceUser := mustCreateUserRecord(t, store, storage.User{
		ID: "user-alice-imp", Name: "alice-imp", CreatedAt: 100, ModifiedAt: 100,
	})
	ws, err := store.LocalWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	mustUpsertMembershipRecord(t, store, storage.Membership{
		UserID: aliceUser.ID, WorkspaceID: ws.ID,
		Role: string(RoleMember), JoinedAt: 100, ModifiedAt: 100,
	})
	created, err := ownerSvc.CreateToken(CreateTokenInput{
		Name:          "imp-agent",
		Type:          "agent",
		Scopes:        []string{"task:read", "impersonate"},
		WorkspaceRefs: []string{"local"},
	})
	if err != nil {
		t.Fatal(err)
	}
	authn, err := ownerSvc.AuthenticateBearerToken(created.RawToken)
	if err != nil {
		t.Fatal(err)
	}
	authorized, err := ownerSvc.AuthorizeTokenRequest(RequestAuthorizationInput{
		Token:              authn,
		RequiredCapability: "task:read",
		RequiredPermission: PermissionTaskRead,
		WorkspaceRef:       "local",
		SubjectUserRef:     "alice-imp",
	})
	if err != nil {
		t.Fatalf("AuthorizeTokenRequest() error = %v", err)
	}
	if authorized.Runtime.ActorUserID != aliceUser.ID {
		t.Fatalf("actor = %q, want %q", authorized.Runtime.ActorUserID, aliceUser.ID)
	}
	if authorized.Runtime.DelegatorTokenID != created.View.ID {
		t.Fatalf("delegator token = %q, want %q", authorized.Runtime.DelegatorTokenID, created.View.ID)
	}
	if authorized.Runtime.DelegatorUserID == "" {
		t.Fatal("delegator user id is empty")
	}
	if authorized.Runtime.Role != RoleMember {
		t.Fatalf("role = %q, want member", authorized.Runtime.Role)
	}
}

func TestAuthorizeTokenRequestImpersonationRejectsUnknownUser(t *testing.T) {
	store := newTestStore(t)
	ownerSvc := newTestServiceWithRuntime(t, store, 100, "local", "local")
	created, err := ownerSvc.CreateToken(CreateTokenInput{
		Name:          "imp-agent",
		Type:          "agent",
		Scopes:        []string{"task:read", "impersonate"},
		WorkspaceRefs: []string{"local"},
	})
	if err != nil {
		t.Fatal(err)
	}
	authn, err := ownerSvc.AuthenticateBearerToken(created.RawToken)
	if err != nil {
		t.Fatal(err)
	}
	_, err = ownerSvc.AuthorizeTokenRequest(RequestAuthorizationInput{
		Token:              authn,
		RequiredCapability: "task:read",
		RequiredPermission: PermissionTaskRead,
		WorkspaceRef:       "local",
		SubjectUserRef:     "nonexistent-user",
	})
	assertRuntimeCode(t, err, "membership_not_found")
}

func TestAuthorizeTokenRequestImpersonationRejectsNonMember(t *testing.T) {
	store := newTestStore(t)
	ownerSvc := newTestServiceWithRuntime(t, store, 100, "local", "local")
	otherUser := mustCreateUserRecord(t, store, storage.User{
		ID: "user-other-imp", Name: "other-imp", CreatedAt: 100, ModifiedAt: 100,
	})
	_ = otherUser
	created, err := ownerSvc.CreateToken(CreateTokenInput{
		Name:          "imp-agent",
		Type:          "agent",
		Scopes:        []string{"task:read", "impersonate"},
		WorkspaceRefs: []string{"local"},
	})
	if err != nil {
		t.Fatal(err)
	}
	authn, err := ownerSvc.AuthenticateBearerToken(created.RawToken)
	if err != nil {
		t.Fatal(err)
	}
	_, err = ownerSvc.AuthorizeTokenRequest(RequestAuthorizationInput{
		Token:              authn,
		RequiredCapability: "task:read",
		RequiredPermission: PermissionTaskRead,
		WorkspaceRef:       "local",
		SubjectUserRef:     "other-imp",
	})
	assertRuntimeCode(t, err, "membership_not_found")
}

func TestAuthorizeTokenRequestImpersonationRejectsWithoutScope(t *testing.T) {
	store := newTestStore(t)
	ownerSvc := newTestServiceWithRuntime(t, store, 100, "local", "local")
	created, err := ownerSvc.CreateToken(CreateTokenInput{
		Name:          "no-impersonate",
		Scopes:        []string{"task:read"},
		WorkspaceRefs: []string{"local"},
	})
	if err != nil {
		t.Fatal(err)
	}
	authn, err := ownerSvc.AuthenticateBearerToken(created.RawToken)
	if err != nil {
		t.Fatal(err)
	}
	_, err = ownerSvc.AuthorizeTokenRequest(RequestAuthorizationInput{
		Token:              authn,
		RequiredCapability: "task:read",
		RequiredPermission: PermissionTaskRead,
		WorkspaceRef:       "local",
		SubjectUserRef:     "local",
	})
	assertRuntimeCode(t, err, "token_scope_denied")
}

func TestAuthorizeTokenRequestImpersonationRejectsPAT(t *testing.T) {
	store := newTestStore(t)
	ownerSvc := newTestServiceWithRuntime(t, store, 100, "local", "local")
	created, err := ownerSvc.CreateToken(CreateTokenInput{
		Name:          "pat-imp",
		Type:          "pat",
		Scopes:        []string{"task:read"},
		WorkspaceRefs: []string{"local"},
	})
	if err != nil {
		t.Fatal(err)
	}
	authn, err := ownerSvc.AuthenticateBearerToken(created.RawToken)
	if err != nil {
		t.Fatal(err)
	}
	_, err = ownerSvc.AuthorizeTokenRequest(RequestAuthorizationInput{
		Token:              authn,
		RequiredCapability: "task:read",
		RequiredPermission: PermissionTaskRead,
		WorkspaceRef:       "local",
		SubjectUserRef:     "local",
	})
	assertRuntimeCode(t, err, "token_scope_denied")
}

func TestAuthorizeTokenRequestImpersonationWorkspaceRequiredMultiWorkspace(t *testing.T) {
	store := newTestStore(t)
	ownerSvc := newTestServiceWithRuntime(t, store, 100, "local", "local")
	_, err := ownerSvc.AddWorkspace(AddWorkspaceInput{Slug: "work-imp", Name: "Work Imp"})
	if err != nil {
		t.Fatal(err)
	}
	created, err := ownerSvc.CreateToken(CreateTokenInput{
		Name:          "multi-ws-agent",
		Type:          "agent",
		Scopes:        []string{"task:read", "impersonate"},
		WorkspaceRefs: []string{"local", "work-imp"},
	})
	if err != nil {
		t.Fatal(err)
	}
	authn, err := ownerSvc.AuthenticateBearerToken(created.RawToken)
	if err != nil {
		t.Fatal(err)
	}
	_, err = ownerSvc.AuthorizeTokenRequest(RequestAuthorizationInput{
		Token:              authn,
		RequiredCapability: "task:read",
		RequiredPermission: PermissionTaskRead,
		SubjectUserRef:     "local",
	})
	assertRuntimeCode(t, err, "workspace_required")
}

func TestImpersonatedTaskActionRecordsDelegatorInAudit(t *testing.T) {
	store := newTestStore(t)
	ownerSvc := newTestServiceWithRuntime(t, store, 100, "local", "local")
	aliceUser := mustCreateUserRecord(t, store, storage.User{
		ID: "user-alice-audit", Name: "alice-audit", CreatedAt: 100, ModifiedAt: 100,
	})
	ws, err := store.LocalWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	mustUpsertMembershipRecord(t, store, storage.Membership{
		UserID: aliceUser.ID, WorkspaceID: ws.ID,
		Role: string(RoleMember), JoinedAt: 100, ModifiedAt: 100,
	})
	created, err := ownerSvc.CreateToken(CreateTokenInput{
		Name:          "audit-agent",
		Type:          "agent",
		Scopes:        []string{"task:write", "audit:read", "impersonate"},
		WorkspaceRefs: []string{"local"},
	})
	if err != nil {
		t.Fatal(err)
	}
	authn, err := ownerSvc.AuthenticateBearerToken(created.RawToken)
	if err != nil {
		t.Fatal(err)
	}
	authorized, err := ownerSvc.AuthorizeTokenRequest(RequestAuthorizationInput{
		Token:              authn,
		RequiredCapability: "task:write",
		RequiredPermission: PermissionTaskWrite,
		WorkspaceRef:       "local",
		SubjectUserRef:     "alice-audit",
	})
	if err != nil {
		t.Fatalf("AuthorizeTokenRequest() error = %v", err)
	}
	impersonatedSvc, err := NewService(ServiceOptions{
		Store:        store,
		Clock:        FixedClock{NowUnix: 100},
		Runtime:      &authorized.Runtime,
		RequestScope: &authorized.Scope,
	})
	if err != nil {
		t.Fatalf("NewService(impersonated) error = %v", err)
	}
	if _, err := impersonatedSvc.Add(AddInput{Description: "impersonated task"}); err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	ownerAuditSvc := newTestServiceWithRuntime(t, store, 100, "local", "local")
	rows, err := ownerAuditSvc.ListAudit(AuditListInput{Limit: 10})
	if err != nil {
		t.Fatalf("ListAudit() error = %v", err)
	}
	found := false
	for _, row := range rows {
		if row.Action == "task.add" && row.TargetType == "task" {
			if row.Actor == nil || row.Actor.ID != aliceUser.ID {
				t.Fatalf("audit actor = %v, want alice id", row.Actor)
			}
			if row.DelegatorTokenID == nil || *row.DelegatorTokenID != created.View.ID {
				t.Fatalf("audit delegator_token_id = %v, want %s", row.DelegatorTokenID, created.View.ID)
			}
			if row.DelegatorUser == nil {
				t.Fatal("audit delegator_user is nil")
			}
			found = true
			break
		}
	}
	if !found {
		t.Fatal("no task.add audit entry found for impersonated task")
	}
}

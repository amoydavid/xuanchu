package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/dajee/taskg/internal/query"
	"github.com/dajee/taskg/internal/storage"
	"github.com/dajee/taskg/internal/task"
	taskrcparser "github.com/dajee/taskg/internal/taskrc"
	"github.com/dajee/taskg/internal/urgency"
)

func strptr(v string) *string { return &v }

func newTestService(t *testing.T, now int64) (*Service, func()) {
	t.Helper()
	store, err := storage.Open(filepath.Join(t.TempDir(), "taskg.db"))
	if err != nil {
		t.Fatal(err)
	}
	svc, err := NewService(ServiceOptions{Store: store, Clock: FixedClock{NowUnix: now}})
	if err != nil {
		t.Fatal(err)
	}
	return svc, func() { _ = store.Close() }
}

func newTestStore(t *testing.T) *storage.Store {
	t.Helper()
	store, err := storage.Open(filepath.Join(t.TempDir(), "taskg.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func newTestServiceWithRuntime(t *testing.T, store *storage.Store, now int64, actorRef, workspaceRef string) *Service {
	t.Helper()
	svc, err := NewService(ServiceOptions{
		Store:        store,
		Clock:        FixedClock{NowUnix: now},
		ActorRef:     actorRef,
		WorkspaceRef: workspaceRef,
	})
	if err != nil {
		t.Fatal(err)
	}
	return svc
}

func mustCreateUserRecord(t *testing.T, store *storage.Store, user storage.User) storage.User {
	t.Helper()
	created, err := storage.NewUserRepository(store.DB()).Create(user)
	if err != nil {
		t.Fatalf("Create(user %s) error = %v", user.Name, err)
	}
	return created
}

func mustCreateWorkspaceRecord(t *testing.T, store *storage.Store, ws storage.Workspace) storage.Workspace {
	t.Helper()
	created, err := storage.NewWorkspaceRepository(store.DB()).Create(ws)
	if err != nil {
		t.Fatalf("Create(workspace %s) error = %v", ws.Slug, err)
	}
	return created
}

func mustUpsertMembershipRecord(t *testing.T, store *storage.Store, member storage.Membership) {
	t.Helper()
	if err := storage.NewMemberRepository(store.DB()).Upsert(member); err != nil {
		t.Fatalf("Upsert(membership %+v) error = %v", member, err)
	}
}

func TestServiceAddListInfo(t *testing.T) {
	store, err := storage.Open(filepath.Join(t.TempDir(), "taskg.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	svc, err := NewService(ServiceOptions{Store: store, Clock: FixedClock{NowUnix: 100}})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	created, err := svc.Add(AddInput{Description: "write spec", Tags: []string{"planning"}})
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	if created.UUID == "" {
		t.Fatal("created UUID is empty")
	}

	tasks, err := svc.List(ListInput{})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(tasks) != 1 || tasks[0].Description != "write spec" {
		t.Fatalf("tasks = %#v", tasks)
	}

	got, err := svc.Info(created.UUID)
	if err != nil {
		t.Fatalf("Info() error = %v", err)
	}
	if got.UUID != created.UUID {
		t.Fatalf("Info UUID = %q, want %q", got.UUID, created.UUID)
	}
}

func TestNewServiceResolvesLocalRuntimeContext(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()

	rt := svc.Runtime()
	if rt.ActorUserID == "" || rt.WorkspaceID == "" {
		t.Fatalf("runtime = %#v", rt)
	}
	if rt.ActorName != "local" {
		t.Fatalf("actor name = %q, want local", rt.ActorName)
	}
	if rt.WorkspaceSlug != "local" {
		t.Fatalf("workspace slug = %q, want local", rt.WorkspaceSlug)
	}
	if rt.Role != RoleOwner {
		t.Fatalf("role = %q, want owner", rt.Role)
	}
}

func TestNewServiceWorkspaceOverride(t *testing.T) {
	store := newTestStore(t)
	userRepo := storage.NewUserRepository(store.DB())
	wsRepo := storage.NewWorkspaceRepository(store.DB())
	memberRepo := storage.NewMemberRepository(store.DB())

	localUser, err := userRepo.GetByName("local")
	if err != nil {
		t.Fatalf("GetByName(local) error = %v", err)
	}
	work, err := wsRepo.Create(storage.Workspace{
		ID:              "ws-work",
		Slug:            "work",
		Name:            "Work",
		CreatedByUserID: &localUser.ID,
		Visibility:      "team",
		SettingsJSON:    "{}",
		CreatedAt:       100,
		ModifiedAt:      100,
	})
	if err != nil {
		t.Fatalf("Create(workspace) error = %v", err)
	}
	if err := memberRepo.Upsert(storage.Membership{
		UserID:      localUser.ID,
		WorkspaceID: work.ID,
		Role:        "admin",
		JoinedAt:    100,
		ModifiedAt:  100,
	}); err != nil {
		t.Fatalf("Upsert(membership) error = %v", err)
	}

	svc, err := NewService(ServiceOptions{
		Store:        store,
		Clock:        FixedClock{NowUnix: 100},
		WorkspaceRef: "work",
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	if got := svc.Runtime().WorkspaceSlug; got != "work" {
		t.Fatalf("workspace slug = %q, want work", got)
	}
	if got := svc.Runtime().Role; got != RoleAdmin {
		t.Fatalf("role = %q, want admin", got)
	}
}

func TestViewerCannotModifyTasks(t *testing.T) {
	store := newTestStore(t)
	ownerSvc := newTestServiceWithRuntime(t, store, 100, "local", "local")
	created, err := ownerSvc.Add(AddInput{Description: "owner task"})
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	userRepo := storage.NewUserRepository(store.DB())
	memberRepo := storage.NewMemberRepository(store.DB())
	ws, err := store.LocalWorkspace()
	if err != nil {
		t.Fatalf("LocalWorkspace() error = %v", err)
	}
	viewer, err := userRepo.Create(storage.User{ID: "user-viewer", Name: "viewer", CreatedAt: 100, ModifiedAt: 100})
	if err != nil {
		t.Fatalf("Create(viewer) error = %v", err)
	}
	if err := memberRepo.Upsert(storage.Membership{
		UserID:      viewer.ID,
		WorkspaceID: ws.ID,
		Role:        string(RoleViewer),
		JoinedAt:    100,
		ModifiedAt:  100,
	}); err != nil {
		t.Fatalf("Upsert(viewer membership) error = %v", err)
	}

	viewerSvc := newTestServiceWithRuntime(t, store, 100, viewer.Name, ws.Slug)
	priority := "H"
	err = viewerSvc.Modify(created.UUID, ModifyInput{Priority: &priority})
	if err == nil {
		t.Fatal("Modify() error = nil, want permission denied")
	}
	permErr, ok := err.(PermissionError)
	if !ok || permErr.Code != "permission_denied" {
		t.Fatalf("err = %#v, want PermissionError(permission_denied)", err)
	}
}

func TestServiceAddResolvesAssigneesInWorkspace(t *testing.T) {
	store := newTestStore(t)
	ownerSvc := newTestServiceWithRuntime(t, store, 100, "local", "local")
	memberUser := mustCreateUserRecord(t, store, storage.User{
		ID:         "user-alice",
		Name:       "alice",
		Email:      strptr("alice@example.com"),
		CreatedAt:  100,
		ModifiedAt: 100,
	})
	ws, err := store.LocalWorkspace()
	if err != nil {
		t.Fatalf("LocalWorkspace() error = %v", err)
	}
	mustUpsertMembershipRecord(t, store, storage.Membership{
		UserID:      memberUser.ID,
		WorkspaceID: ws.ID,
		Role:        string(RoleMember),
		JoinedAt:    100,
		ModifiedAt:  100,
	})

	created, err := ownerSvc.Add(AddInput{
		Description: "write spec",
		Assignees:   []string{"alice"},
	})
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	if got := created.Assignees; len(got) != 1 || got[0].UserID != memberUser.ID || got[0].Name != "alice" {
		t.Fatalf("Assignees = %#v", got)
	}
}

func TestServiceModifyRejectsCrossWorkspaceAssignee(t *testing.T) {
	store := newTestStore(t)
	localSvc := newTestServiceWithRuntime(t, store, 100, "local", "local")
	otherWS := mustCreateWorkspaceRecord(t, store, storage.Workspace{
		ID:           "ws-other",
		Slug:         "other",
		Name:         "Other",
		Visibility:   "team",
		SettingsJSON: "{}",
		CreatedAt:    100,
		ModifiedAt:   100,
	})
	otherUser := mustCreateUserRecord(t, store, storage.User{
		ID:         "user-other",
		Name:       "other-user",
		CreatedAt:  100,
		ModifiedAt: 100,
	})
	mustUpsertMembershipRecord(t, store, storage.Membership{
		UserID:      otherUser.ID,
		WorkspaceID: otherWS.ID,
		Role:        string(RoleMember),
		JoinedAt:    100,
		ModifiedAt:  100,
	})
	created, err := localSvc.Add(AddInput{Description: "write spec"})
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	err = localSvc.Modify(created.UUID, ModifyInput{AddAssignees: []string{"other-user"}})
	if err == nil {
		t.Fatal("Modify() error = nil, want assignee_not_member")
	}
	rtErr, ok := err.(RuntimeError)
	if !ok || rtErr.Code != "assignee_not_member" {
		t.Fatalf("Modify() err = %#v, want RuntimeError(assignee_not_member)", err)
	}
}

func TestServiceListExpandsAssigneeMe(t *testing.T) {
	store := newTestStore(t)
	svc := newTestServiceWithRuntime(t, store, 100, "local", "local")
	localUser, err := storage.NewUserRepository(store.DB()).GetByName("local")
	if err != nil {
		t.Fatalf("GetByName(local) error = %v", err)
	}
	mine, err := svc.Add(AddInput{
		Description: "my task",
		Assignees:   []string{localUser.ID},
	})
	if err != nil {
		t.Fatalf("Add(my task) error = %v", err)
	}
	if _, err := svc.Add(AddInput{Description: "other task"}); err != nil {
		t.Fatalf("Add(other task) error = %v", err)
	}

	expr, err := query.ParseQuery(`assignee:me`)
	if err != nil {
		t.Fatalf("ParseQuery() error = %v", err)
	}
	tasks, err := svc.List(ListInput{Query: expr, NoContext: true})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(tasks) != 1 || tasks[0].UUID != mine.UUID {
		t.Fatalf("tasks = %#v, want only mine", tasks)
	}
}

func TestServiceContextFilterResolvesAssigneeMe(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()
	mine, err := svc.Add(AddInput{Description: "my context task", Assignees: []string{"local"}})
	if err != nil {
		t.Fatalf("Add(my context task) error = %v", err)
	}
	if _, err := svc.Add(AddInput{Description: "other context task"}); err != nil {
		t.Fatalf("Add(other context task) error = %v", err)
	}
	if err := svc.DefineContext("mine", "assignee:me"); err != nil {
		t.Fatalf("DefineContext(mine) error = %v", err)
	}
	if err := svc.UseContext("mine"); err != nil {
		t.Fatalf("UseContext(mine) error = %v", err)
	}

	tasks, err := svc.List(ListInput{})
	if err != nil {
		t.Fatalf("List() with assignee context error = %v", err)
	}
	if len(tasks) != 1 || tasks[0].UUID != mine.UUID {
		t.Fatalf("List() with assignee context = %#v, want only mine", tasks)
	}

	all, err := svc.RunReport(ReportInput{Name: "all"})
	if err != nil {
		t.Fatalf("RunReport(all) with assignee context error = %v", err)
	}
	if len(all.Tasks) != 1 || all.Tasks[0].UUID != mine.UUID {
		t.Fatalf("RunReport(all) with assignee context = %#v, want only mine", all.Tasks)
	}
}

func TestServiceModifyClearAndAddAssigneesReplacesSet(t *testing.T) {
	store := newTestStore(t)
	svc := newTestServiceWithRuntime(t, store, 100, "local", "local")
	alice := mustCreateUserRecord(t, store, storage.User{
		ID:         "user-alice-clear-add",
		Name:       "alice-clear-add",
		Email:      strptr("alice-clear-add@example.com"),
		CreatedAt:  100,
		ModifiedAt: 100,
	})
	ws, err := store.LocalWorkspace()
	if err != nil {
		t.Fatalf("LocalWorkspace() error = %v", err)
	}
	mustUpsertMembershipRecord(t, store, storage.Membership{
		UserID:      alice.ID,
		WorkspaceID: ws.ID,
		Role:        string(RoleMember),
		JoinedAt:    100,
		ModifiedAt:  100,
	})
	created, err := svc.Add(AddInput{Description: "replace assignees", Assignees: []string{"local"}})
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	if err := svc.Modify(created.UUID, ModifyInput{ClearAssignees: true, AddAssignees: []string{"alice-clear-add"}}); err != nil {
		t.Fatalf("Modify(clear+add assignees) error = %v", err)
	}
	got, err := svc.Info(created.UUID)
	if err != nil {
		t.Fatalf("Info() error = %v", err)
	}
	if len(got.Assignees) != 1 || got.Assignees[0].UserID != alice.ID {
		t.Fatalf("Assignees after clear+add = %#v, want only alice", got.Assignees)
	}
}

func TestServiceBindAndUnbindExternalID(t *testing.T) {
	store := newTestStore(t)
	svc := newTestServiceWithRuntime(t, store, 100, "local", "local")

	err := svc.BindExternalID(svc.runtime.ActorUserID, "feishu", "ou_test_bind")
	if err != nil {
		t.Fatalf("BindExternalID() error = %v", err)
	}

	extIDs, err := svc.ListExternalIDs(svc.runtime.ActorUserID)
	if err != nil {
		t.Fatalf("ListExternalIDs() error = %v", err)
	}
	if len(extIDs) != 1 || extIDs[0].Provider != "feishu" || extIDs[0].ExternalID != "ou_test_bind" {
		t.Fatalf("external IDs = %#v, want [feishu:ou_test_bind]", extIDs)
	}

	err = svc.UnbindExternalID(svc.runtime.ActorUserID, "feishu", "ou_test_bind")
	if err != nil {
		t.Fatalf("UnbindExternalID() error = %v", err)
	}

	extIDs, err = svc.ListExternalIDs(svc.runtime.ActorUserID)
	if err != nil {
		t.Fatalf("ListExternalIDs() error = %v", err)
	}
	if len(extIDs) != 0 {
		t.Fatalf("external IDs after unbind = %#v, want empty", extIDs)
	}
}

func TestServiceBindExternalIDRejectsDuplicate(t *testing.T) {
	store := newTestStore(t)
	svc := newTestServiceWithRuntime(t, store, 100, "local", "local")

	err := svc.BindExternalID(svc.runtime.ActorUserID, "feishu", "ou_dup")
	if err != nil {
		t.Fatalf("first bind: %v", err)
	}
	err = svc.BindExternalID(svc.runtime.ActorUserID, "feishu", "ou_dup")
	if err == nil {
		t.Fatal("expected duplicate bind to fail")
	}
}

func TestServiceUnbindExternalIDNotFound(t *testing.T) {
	store := newTestStore(t)
	svc := newTestServiceWithRuntime(t, store, 100, "local", "local")

	err := svc.UnbindExternalID(svc.runtime.ActorUserID, "feishu", "ou_nonexist")
	if err == nil {
		t.Fatal("expected unbind of non-existent ID to fail")
	}
}

func TestServiceBindExternalIDRejectsOtherUserForNonAdmin(t *testing.T) {
	store := newTestStore(t)
	adminSvc := newTestServiceWithRuntime(t, store, 100, "local", "local")

	otherUser := mustCreateUserRecord(t, store, storage.User{
		ID: "user-other", Name: "other", CreatedAt: 100, ModifiedAt: 100,
	})
	ws, err := store.LocalWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	mustUpsertMembershipRecord(t, store, storage.Membership{
		UserID: otherUser.ID, WorkspaceID: ws.ID, Role: "member", JoinedAt: 100, ModifiedAt: 100,
	})

	memberSvc := newTestServiceWithRuntime(t, store, 200, otherUser.Name, ws.Slug)

	err = memberSvc.BindExternalID(adminSvc.runtime.ActorUserID, "feishu", "ou_other")
	if err == nil {
		t.Fatal("expected non-admin binding other user to fail")
	}
}

func TestServiceAddResolvesAssigneeByExternalID(t *testing.T) {
	store := newTestStore(t)
	svc := newTestServiceWithRuntime(t, store, 100, "local", "local")

	err := svc.BindExternalID(svc.runtime.ActorUserID, "feishu", "ou_ext_assign")
	if err != nil {
		t.Fatalf("BindExternalID() error = %v", err)
	}

	created, err := svc.Add(AddInput{
		Description: "external assign test",
		Assignees:   []string{"feishu:ou_ext_assign"},
	})
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	got, err := svc.Info(created.UUID)
	if err != nil {
		t.Fatalf("Info() error = %v", err)
	}
	if len(got.Assignees) != 1 || got.Assignees[0].UserID != svc.runtime.ActorUserID {
		t.Fatalf("Assignees = %#v, want current user", got.Assignees)
	}
}

func TestViewerCanUseOwnContextButCannotDefineContext(t *testing.T) {
	store := newTestStore(t)
	ownerSvc := newTestServiceWithRuntime(t, store, 100, "local", "local")
	if err := ownerSvc.DefineContext("work", "project:work"); err != nil {
		t.Fatalf("DefineContext(owner) error = %v", err)
	}

	userRepo := storage.NewUserRepository(store.DB())
	memberRepo := storage.NewMemberRepository(store.DB())
	ws, err := store.LocalWorkspace()
	if err != nil {
		t.Fatalf("LocalWorkspace() error = %v", err)
	}
	viewer, err := userRepo.Create(storage.User{ID: "user-viewer-ctx", Name: "viewer-ctx", CreatedAt: 100, ModifiedAt: 100})
	if err != nil {
		t.Fatalf("Create(viewer) error = %v", err)
	}
	if err := memberRepo.Upsert(storage.Membership{
		UserID:      viewer.ID,
		WorkspaceID: ws.ID,
		Role:        string(RoleViewer),
		JoinedAt:    100,
		ModifiedAt:  100,
	}); err != nil {
		t.Fatalf("Upsert(viewer membership) error = %v", err)
	}

	viewerSvc := newTestServiceWithRuntime(t, store, 100, viewer.Name, ws.Slug)
	if err := viewerSvc.UseContext("work"); err != nil {
		t.Fatalf("UseContext(viewer) error = %v", err)
	}
	show, err := viewerSvc.ContextShow()
	if err != nil || !strings.Contains(show, "work") {
		t.Fatalf("ContextShow() = %q, %v", show, err)
	}
	err = viewerSvc.DefineContext("viewer-only", "project:viewer")
	if err == nil {
		t.Fatal("DefineContext(viewer) error = nil, want permission denied")
	}
	permErr, ok := err.(PermissionError)
	if !ok || permErr.Code != "permission_denied" {
		t.Fatalf("err = %#v, want PermissionError(permission_denied)", err)
	}
}

func TestViewerCannotManageUDASchema(t *testing.T) {
	store := newTestStore(t)
	userRepo := storage.NewUserRepository(store.DB())
	memberRepo := storage.NewMemberRepository(store.DB())
	ws, err := store.LocalWorkspace()
	if err != nil {
		t.Fatalf("LocalWorkspace() error = %v", err)
	}
	viewer, err := userRepo.Create(storage.User{ID: "user-viewer-uda", Name: "viewer-uda", CreatedAt: 100, ModifiedAt: 100})
	if err != nil {
		t.Fatalf("Create(viewer) error = %v", err)
	}
	if err := memberRepo.Upsert(storage.Membership{
		UserID:      viewer.ID,
		WorkspaceID: ws.ID,
		Role:        string(RoleViewer),
		JoinedAt:    100,
		ModifiedAt:  100,
	}); err != nil {
		t.Fatalf("Upsert(viewer membership) error = %v", err)
	}

	viewerSvc := newTestServiceWithRuntime(t, store, 100, viewer.Name, ws.Slug)
	err = viewerSvc.DefineUDA("estimate", "numeric", "Estimate", nil, "")
	if err == nil {
		t.Fatal("DefineUDA() error = nil, want permission denied")
	}
	permErr, ok := err.(PermissionError)
	if !ok || permErr.Code != "permission_denied" {
		t.Fatalf("err = %#v, want PermissionError(permission_denied)", err)
	}
}

func TestMemberCannotManageWorkspaceMetadataOrMembers(t *testing.T) {
	store := newTestStore(t)
	userRepo := storage.NewUserRepository(store.DB())
	memberRepo := storage.NewMemberRepository(store.DB())
	ws, err := store.LocalWorkspace()
	if err != nil {
		t.Fatalf("LocalWorkspace() error = %v", err)
	}
	member, err := userRepo.Create(storage.User{ID: "user-member", Name: "member-user", CreatedAt: 100, ModifiedAt: 100})
	if err != nil {
		t.Fatalf("Create(member) error = %v", err)
	}
	if err := memberRepo.Upsert(storage.Membership{
		UserID:      member.ID,
		WorkspaceID: ws.ID,
		Role:        string(RoleMember),
		JoinedAt:    100,
		ModifiedAt:  100,
	}); err != nil {
		t.Fatalf("Upsert(member membership) error = %v", err)
	}

	memberSvc := newTestServiceWithRuntime(t, store, 100, member.Name, ws.Slug)
	if err := memberSvc.Require(PermissionWorkspaceModify); err == nil {
		t.Fatal("Require(PermissionWorkspaceModify) error = nil, want denied")
	}
	if err := memberSvc.Require(PermissionMemberManage); err == nil {
		t.Fatal("Require(PermissionMemberManage) error = nil, want denied")
	}
}

func TestAdminCannotArchiveWorkspace(t *testing.T) {
	store := newTestStore(t)
	userRepo := storage.NewUserRepository(store.DB())
	memberRepo := storage.NewMemberRepository(store.DB())
	ws, err := store.LocalWorkspace()
	if err != nil {
		t.Fatalf("LocalWorkspace() error = %v", err)
	}
	admin, err := userRepo.Create(storage.User{ID: "user-admin", Name: "admin-user", CreatedAt: 100, ModifiedAt: 100})
	if err != nil {
		t.Fatalf("Create(admin) error = %v", err)
	}
	if err := memberRepo.Upsert(storage.Membership{
		UserID:      admin.ID,
		WorkspaceID: ws.ID,
		Role:        string(RoleAdmin),
		JoinedAt:    100,
		ModifiedAt:  100,
	}); err != nil {
		t.Fatalf("Upsert(admin membership) error = %v", err)
	}

	adminSvc := newTestServiceWithRuntime(t, store, 100, admin.Name, ws.Slug)
	if err := adminSvc.Require(PermissionWorkspaceArchive); err == nil {
		t.Fatal("Require(PermissionWorkspaceArchive) error = nil, want denied")
	}
}

func TestProjectPermissionsByRole(t *testing.T) {
	store := newTestStore(t)
	ownerSvc := newTestServiceWithRuntime(t, store, 100, "local", "local")
	adminUser := mustCreateUserRecord(t, store, storage.User{ID: "user-project-admin", Name: "project-admin", CreatedAt: 100, ModifiedAt: 100})
	memberUser := mustCreateUserRecord(t, store, storage.User{ID: "user-project-member", Name: "project-member", CreatedAt: 100, ModifiedAt: 100})
	viewerUser := mustCreateUserRecord(t, store, storage.User{ID: "user-project-viewer", Name: "project-viewer", CreatedAt: 100, ModifiedAt: 100})
	ws, err := store.LocalWorkspace()
	if err != nil {
		t.Fatalf("LocalWorkspace() error = %v", err)
	}
	for _, member := range []storage.Membership{
		{UserID: adminUser.ID, WorkspaceID: ws.ID, Role: string(RoleAdmin), JoinedAt: 100, ModifiedAt: 100},
		{UserID: memberUser.ID, WorkspaceID: ws.ID, Role: string(RoleMember), JoinedAt: 100, ModifiedAt: 100},
		{UserID: viewerUser.ID, WorkspaceID: ws.ID, Role: string(RoleViewer), JoinedAt: 100, ModifiedAt: 100},
	} {
		mustUpsertMembershipRecord(t, store, member)
	}

	if err := ownerSvc.Require(PermissionProjectManage); err != nil {
		t.Fatalf("owner Require(PermissionProjectManage) error = %v", err)
	}
	if err := ownerSvc.Require(PermissionProjectConfigWrite); err != nil {
		t.Fatalf("owner Require(PermissionProjectConfigWrite) error = %v", err)
	}

	adminSvc := newTestServiceWithRuntime(t, store, 100, adminUser.Name, ws.Slug)
	created, err := adminSvc.AddProject(AddProjectInput{Slug: "api", Name: "API"})
	if err != nil {
		t.Fatalf("admin AddProject() error = %v", err)
	}
	renamed := "API Platform"
	if err := adminSvc.ModifyProject(created.ID, ModifyProjectInput{Name: &renamed}); err != nil {
		t.Fatalf("admin ModifyProject() error = %v", err)
	}
	if _, err := adminSvc.ArchiveProject(created.ID); err != nil {
		t.Fatalf("admin ArchiveProject() error = %v", err)
	}

	ownerCreated, err := ownerSvc.AddProject(AddProjectInput{Slug: "ops", Name: "Ops"})
	if err != nil {
		t.Fatalf("owner AddProject() error = %v", err)
	}
	memberSvc := newTestServiceWithRuntime(t, store, 100, memberUser.Name, ws.Slug)
	if err := memberSvc.Require(PermissionProjectRead); err != nil {
		t.Fatalf("member Require(PermissionProjectRead) error = %v", err)
	}
	if err := memberSvc.Require(PermissionProjectConfigRead); err != nil {
		t.Fatalf("member Require(PermissionProjectConfigRead) error = %v", err)
	}
	if _, err := memberSvc.ListProjects(true); err != nil {
		t.Fatalf("member ListProjects() error = %v", err)
	}
	if _, err := memberSvc.ProjectInfo(ownerCreated.ID); err != nil {
		t.Fatalf("member ProjectInfo() error = %v", err)
	}
	if _, err := memberSvc.AddProject(AddProjectInput{Slug: "member-write", Name: "Member Write"}); err == nil {
		t.Fatal("member AddProject() error = nil, want permission denied")
	} else if permErr, ok := err.(PermissionError); !ok || permErr.Code != "permission_denied" {
		t.Fatalf("member AddProject() err = %#v, want PermissionError(permission_denied)", err)
	}
	if err := memberSvc.ModifyProject(ownerCreated.ID, ModifyProjectInput{Name: &renamed}); err == nil {
		t.Fatal("member ModifyProject() error = nil, want permission denied")
	} else if permErr, ok := err.(PermissionError); !ok || permErr.Code != "permission_denied" {
		t.Fatalf("member ModifyProject() err = %#v, want PermissionError(permission_denied)", err)
	}
	if _, err := memberSvc.ArchiveProject(ownerCreated.ID); err == nil {
		t.Fatal("member ArchiveProject() error = nil, want permission denied")
	} else if permErr, ok := err.(PermissionError); !ok || permErr.Code != "permission_denied" {
		t.Fatalf("member ArchiveProject() err = %#v, want PermissionError(permission_denied)", err)
	}

	viewerSvc := newTestServiceWithRuntime(t, store, 100, viewerUser.Name, ws.Slug)
	if err := viewerSvc.Require(PermissionProjectRead); err != nil {
		t.Fatalf("viewer Require(PermissionProjectRead) error = %v", err)
	}
	if err := viewerSvc.Require(PermissionProjectConfigRead); err != nil {
		t.Fatalf("viewer Require(PermissionProjectConfigRead) error = %v", err)
	}
	if _, err := viewerSvc.ListProjects(true); err != nil {
		t.Fatalf("viewer ListProjects() error = %v", err)
	}
	if _, err := viewerSvc.ProjectInfo(ownerCreated.Slug); err != nil {
		t.Fatalf("viewer ProjectInfo() error = %v", err)
	}
	if _, err := viewerSvc.AddProject(AddProjectInput{Slug: "viewer-write", Name: "Viewer Write"}); err == nil {
		t.Fatal("viewer AddProject() error = nil, want permission denied")
	} else if permErr, ok := err.(PermissionError); !ok || permErr.Code != "permission_denied" {
		t.Fatalf("viewer AddProject() err = %#v, want PermissionError(permission_denied)", err)
	}
	if err := viewerSvc.ModifyProject(ownerCreated.ID, ModifyProjectInput{Name: &renamed}); err == nil {
		t.Fatal("viewer ModifyProject() error = nil, want permission denied")
	} else if permErr, ok := err.(PermissionError); !ok || permErr.Code != "permission_denied" {
		t.Fatalf("viewer ModifyProject() err = %#v, want PermissionError(permission_denied)", err)
	}
	if _, err := viewerSvc.ArchiveProject(ownerCreated.ID); err == nil {
		t.Fatal("viewer ArchiveProject() error = nil, want permission denied")
	} else if permErr, ok := err.(PermissionError); !ok || permErr.Code != "permission_denied" {
		t.Fatalf("viewer ArchiveProject() err = %#v, want PermissionError(permission_denied)", err)
	}
}

func TestProjectInfoRejectsWorkspaceMismatchInRuntime(t *testing.T) {
	store := newTestStore(t)
	ownerLocal := newTestServiceWithRuntime(t, store, 100, "local", "local")
	work, err := ownerLocal.AddWorkspace(AddWorkspaceInput{Slug: "work", Name: "Work"})
	if err != nil {
		t.Fatalf("AddWorkspace(work) error = %v", err)
	}
	ownerWork := newTestServiceWithRuntime(t, store, 100, "local", work.Slug)
	created, err := ownerWork.AddProject(AddProjectInput{Slug: "api", Name: "API"})
	if err != nil {
		t.Fatalf("AddProject(work/api) error = %v", err)
	}

	_, err = ownerLocal.ProjectInfo(created.ID)
	if err == nil {
		t.Fatal("ProjectInfo(cross workspace id) error = nil, want project_workspace_mismatch")
	}
	runtimeErr, ok := err.(RuntimeError)
	if !ok || runtimeErr.Code != "project_workspace_mismatch" {
		t.Fatalf("ProjectInfo(cross workspace id) err = %#v, want RuntimeError(project_workspace_mismatch)", err)
	}
}

func TestProjectAllowsSameSlugAcrossWorkspaces(t *testing.T) {
	store := newTestStore(t)
	ownerLocal := newTestServiceWithRuntime(t, store, 100, "local", "local")
	if _, err := ownerLocal.AddProject(AddProjectInput{Slug: "api", Name: "Local API"}); err != nil {
		t.Fatalf("local AddProject(api) error = %v", err)
	}
	work, err := ownerLocal.AddWorkspace(AddWorkspaceInput{Slug: "work", Name: "Work"})
	if err != nil {
		t.Fatalf("AddWorkspace(work) error = %v", err)
	}
	ownerWork := newTestServiceWithRuntime(t, store, 100, "local", work.Slug)
	if _, err := ownerWork.AddProject(AddProjectInput{Slug: "api", Name: "Work API"}); err != nil {
		t.Fatalf("work AddProject(api) error = %v", err)
	}
}

func TestArchiveProjectReturnsProjectArchivedWhenAlreadyArchived(t *testing.T) {
	store := newTestStore(t)
	svc := newTestServiceWithRuntime(t, store, 100, "local", "local")
	created, err := svc.AddProject(AddProjectInput{Slug: "api", Name: "API"})
	if err != nil {
		t.Fatalf("AddProject() error = %v", err)
	}
	if _, err := svc.ArchiveProject(created.ID); err != nil {
		t.Fatalf("ArchiveProject(first) error = %v", err)
	}
	if _, err := svc.ArchiveProject(created.ID); err == nil {
		t.Fatal("ArchiveProject(second) error = nil, want project_archived")
	} else if runtimeErr, ok := err.(RuntimeError); !ok || runtimeErr.Code != "project_archived" {
		t.Fatalf("ArchiveProject(second) err = %#v, want RuntimeError(project_archived)", err)
	}
}

func TestListProjectsCountsArchivedProjectTasksExcludingDeleted(t *testing.T) {
	store := newTestStore(t)
	svc := newTestServiceWithRuntime(t, store, 100, "local", "local")
	created, err := svc.AddProject(AddProjectInput{Slug: "api", Name: "API"})
	if err != nil {
		t.Fatalf("AddProject() error = %v", err)
	}
	projectRef := created.Slug
	if _, err := svc.Add(AddInput{Description: "pending task", Project: &projectRef}); err != nil {
		t.Fatalf("Add(pending task) error = %v", err)
	}
	doneTask, err := svc.Add(AddInput{Description: "done task", Project: &projectRef})
	if err != nil {
		t.Fatalf("Add(done task) error = %v", err)
	}
	deletedTask, err := svc.Add(AddInput{Description: "deleted task", Project: &projectRef})
	if err != nil {
		t.Fatalf("Add(deleted task) error = %v", err)
	}
	if err := svc.Done(doneTask.UUID); err != nil {
		t.Fatalf("Done(done task) error = %v", err)
	}
	if err := svc.Delete(deletedTask.UUID); err != nil {
		t.Fatalf("Delete(deleted task) error = %v", err)
	}
	if _, err := svc.ArchiveProject(created.ID); err != nil {
		t.Fatalf("ArchiveProject() error = %v", err)
	}

	projects, err := svc.ListProjects(true)
	if err != nil {
		t.Fatalf("ListProjects(includeArchived) error = %v", err)
	}
	if len(projects) != 1 {
		t.Fatalf("ListProjects(includeArchived) = %#v, want 1 project", projects)
	}
	if projects[0].TaskCount != 2 {
		t.Fatalf("archived project TaskCount = %d, want 2", projects[0].TaskCount)
	}
}

func TestProjectAuditEntriesIncludeProjectID(t *testing.T) {
	store := newTestStore(t)
	svc := newTestServiceWithRuntime(t, store, 100, "local", "local")
	created, err := svc.AddProject(AddProjectInput{Slug: "api", Name: "API"})
	if err != nil {
		t.Fatalf("AddProject() error = %v", err)
	}
	description := "updated"
	if err := svc.ModifyProject(created.ID, ModifyProjectInput{Description: &description}); err != nil {
		t.Fatalf("ModifyProject() error = %v", err)
	}
	if _, err := svc.ArchiveProject(created.ID); err != nil {
		t.Fatalf("ArchiveProject() error = %v", err)
	}

	logs, err := svc.ListAudit(AuditListInput{Limit: 10})
	if err != nil {
		t.Fatalf("ListAudit() error = %v", err)
	}
	if len(logs) < 3 {
		t.Fatalf("ListAudit() rows = %#v, want >= 3", logs)
	}
	for i := 0; i < 3; i++ {
		if logs[i].TargetID != created.ID {
			t.Fatalf("logs[%d].TargetID = %q, want %q", i, logs[i].TargetID, created.ID)
		}
	}
	workspaceID := svc.Runtime().WorkspaceID
	rows, err := storage.NewAuditRepository(store.DB()).List(storage.AuditListOptions{
		WorkspaceID: &workspaceID,
		ProjectID:   &created.ID,
		Limit:       10,
	})
	if err != nil {
		t.Fatalf("AuditRepository.List(project filter) error = %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("AuditRepository.List(project filter) rows = %#v, want 3", rows)
	}
	for i, row := range rows {
		if row.ProjectID == nil || *row.ProjectID != created.ID {
			t.Fatalf("rows[%d].ProjectID = %#v, want %q", i, row.ProjectID, created.ID)
		}
	}
}

func TestProjectConfigPermissionsAndArchivedBehavior(t *testing.T) {
	store := newTestStore(t)
	ownerSvc := newTestServiceWithRuntime(t, store, 100, "local", "local")
	ws, err := store.LocalWorkspace()
	if err != nil {
		t.Fatalf("LocalWorkspace() error = %v", err)
	}
	adminUser := mustCreateUserRecord(t, store, storage.User{ID: "user-project-config-admin", Name: "project-config-admin", CreatedAt: 100, ModifiedAt: 100})
	memberUser := mustCreateUserRecord(t, store, storage.User{ID: "user-project-config-member", Name: "project-config-member", CreatedAt: 100, ModifiedAt: 100})
	viewerUser := mustCreateUserRecord(t, store, storage.User{ID: "user-project-config-viewer", Name: "project-config-viewer", CreatedAt: 100, ModifiedAt: 100})
	for _, member := range []storage.Membership{
		{UserID: adminUser.ID, WorkspaceID: ws.ID, Role: string(RoleAdmin), JoinedAt: 100, ModifiedAt: 100},
		{UserID: memberUser.ID, WorkspaceID: ws.ID, Role: string(RoleMember), JoinedAt: 100, ModifiedAt: 100},
		{UserID: viewerUser.ID, WorkspaceID: ws.ID, Role: string(RoleViewer), JoinedAt: 100, ModifiedAt: 100},
	} {
		mustUpsertMembershipRecord(t, store, member)
	}

	project, err := ownerSvc.AddProject(AddProjectInput{Slug: "api", Name: "API"})
	if err != nil {
		t.Fatalf("AddProject() error = %v", err)
	}

	adminSvc := newTestServiceWithRuntime(t, store, 100, adminUser.Name, ws.Slug)
	if err := adminSvc.ProjectConfigSet(project.ID, "agent.background", "Admin background"); err != nil {
		t.Fatalf("admin ProjectConfigSet() error = %v", err)
	}
	got, ok, err := adminSvc.ProjectConfigGet(project.Slug, "agent.background")
	if err != nil {
		t.Fatalf("admin ProjectConfigGet() error = %v", err)
	}
	if !ok || got != "Admin background" {
		t.Fatalf("admin ProjectConfigGet() = (%q, %v), want (%q, true)", got, ok, "Admin background")
	}

	if err := ownerSvc.ProjectConfigSet(project.Slug, "agent.constraints", "Owner constraints"); err != nil {
		t.Fatalf("owner ProjectConfigSet() error = %v", err)
	}
	configs, err := ownerSvc.ProjectConfigList(project.ID)
	if err != nil {
		t.Fatalf("owner ProjectConfigList() error = %v", err)
	}
	if got := configs["agent.background"]; got != "Admin background" {
		t.Fatalf("ProjectConfigList()[agent.background] = %q, want %q", got, "Admin background")
	}
	if got := configs["agent.constraints"]; got != "Owner constraints" {
		t.Fatalf("ProjectConfigList()[agent.constraints] = %q, want %q", got, "Owner constraints")
	}
	if err := ownerSvc.ProjectConfigUnset(project.ID, "agent.constraints"); err != nil {
		t.Fatalf("owner ProjectConfigUnset() error = %v", err)
	}
	if _, ok, err := ownerSvc.ProjectConfigGet(project.ID, "agent.constraints"); err != nil {
		t.Fatalf("owner ProjectConfigGet(unset) error = %v", err)
	} else if ok {
		t.Fatal("owner ProjectConfigGet(unset) found value, want missing")
	}

	memberSvc := newTestServiceWithRuntime(t, store, 100, memberUser.Name, ws.Slug)
	if got, ok, err := memberSvc.ProjectConfigGet(project.ID, "agent.background"); err != nil {
		t.Fatalf("member ProjectConfigGet() error = %v", err)
	} else if !ok || got != "Admin background" {
		t.Fatalf("member ProjectConfigGet() = (%q, %v), want (%q, true)", got, ok, "Admin background")
	}
	if configs, err := memberSvc.ProjectConfigList(project.Slug); err != nil {
		t.Fatalf("member ProjectConfigList() error = %v", err)
	} else if got := configs["agent.background"]; got != "Admin background" {
		t.Fatalf("member ProjectConfigList()[agent.background] = %q, want %q", got, "Admin background")
	}
	if err := memberSvc.ProjectConfigSet(project.ID, "agent.background", "member write"); err == nil {
		t.Fatal("member ProjectConfigSet() error = nil, want permission denied")
	} else if permErr, ok := err.(PermissionError); !ok || permErr.Code != "permission_denied" {
		t.Fatalf("member ProjectConfigSet() err = %#v, want PermissionError(permission_denied)", err)
	}

	viewerSvc := newTestServiceWithRuntime(t, store, 100, viewerUser.Name, ws.Slug)
	if got, ok, err := viewerSvc.ProjectConfigGet(project.Slug, "agent.background"); err != nil {
		t.Fatalf("viewer ProjectConfigGet() error = %v", err)
	} else if !ok || got != "Admin background" {
		t.Fatalf("viewer ProjectConfigGet() = (%q, %v), want (%q, true)", got, ok, "Admin background")
	}
	if configs, err := viewerSvc.ProjectConfigList(project.ID); err != nil {
		t.Fatalf("viewer ProjectConfigList() error = %v", err)
	} else if got := configs["agent.background"]; got != "Admin background" {
		t.Fatalf("viewer ProjectConfigList()[agent.background] = %q, want %q", got, "Admin background")
	}
	if err := viewerSvc.ProjectConfigSet(project.Slug, "agent.background", "viewer write"); err == nil {
		t.Fatal("viewer ProjectConfigSet() error = nil, want permission denied")
	} else if permErr, ok := err.(PermissionError); !ok || permErr.Code != "permission_denied" {
		t.Fatalf("viewer ProjectConfigSet() err = %#v, want PermissionError(permission_denied)", err)
	}

	if _, err := ownerSvc.ArchiveProject(project.ID); err != nil {
		t.Fatalf("ArchiveProject() error = %v", err)
	}
	if got, ok, err := ownerSvc.ProjectConfigGet(project.ID, "agent.background"); err != nil {
		t.Fatalf("ProjectConfigGet(archived) error = %v", err)
	} else if !ok || got != "Admin background" {
		t.Fatalf("ProjectConfigGet(archived) = (%q, %v), want (%q, true)", got, ok, "Admin background")
	}
	if configs, err := ownerSvc.ProjectConfigList(project.Slug); err != nil {
		t.Fatalf("ProjectConfigList(archived) error = %v", err)
	} else if got := configs["agent.background"]; got != "Admin background" {
		t.Fatalf("ProjectConfigList(archived)[agent.background] = %q, want %q", got, "Admin background")
	}
	if err := ownerSvc.ProjectConfigSet(project.ID, "context.default", "project:api"); err == nil {
		t.Fatal("ProjectConfigSet(archived) error = nil, want project_archived")
	} else if runtimeErr, ok := err.(RuntimeError); !ok || runtimeErr.Code != "project_archived" {
		t.Fatalf("ProjectConfigSet(archived) err = %#v, want RuntimeError(project_archived)", err)
	}
	if err := adminSvc.ProjectConfigUnset(project.Slug, "agent.background"); err == nil {
		t.Fatal("ProjectConfigUnset(archived) error = nil, want project_archived")
	} else if runtimeErr, ok := err.(RuntimeError); !ok || runtimeErr.Code != "project_archived" {
		t.Fatalf("ProjectConfigUnset(archived) err = %#v, want RuntimeError(project_archived)", err)
	}
}

func TestConfigScopeRequiresProjectForProjectConfigKeys(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()

	cases := []struct {
		name string
		call func() error
	}{
		{
			name: "set",
			call: func() error {
				return svc.SetConfig("agent.background", "Background")
			},
		},
		{
			name: "unset",
			call: func() error {
				return svc.UnsetConfig("agent.background")
			},
		},
		{
			name: "get",
			call: func() error {
				_, _, err := svc.GetConfig("agent.background")
				return err
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.call()
			if err == nil {
				t.Fatalf("%s error = nil, want project_config_scope_required", tc.name)
			}
			runtimeErr, ok := err.(RuntimeError)
			if !ok || runtimeErr.Code != "project_config_scope_required" {
				t.Fatalf("%s err = %#v, want RuntimeError(project_config_scope_required)", tc.name, err)
			}
		})
	}
}

func TestProjectConfigRejectsUnknownKeyWithStableCode(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()

	project, err := svc.AddProject(AddProjectInput{Slug: "api", Name: "API"})
	if err != nil {
		t.Fatalf("AddProject() error = %v", err)
	}

	cases := []struct {
		name string
		call func() error
	}{
		{
			name: "get",
			call: func() error {
				_, _, err := svc.ProjectConfigGet(project.ID, "unknown.key")
				return err
			},
		},
		{
			name: "set",
			call: func() error {
				return svc.ProjectConfigSet(project.ID, "unknown.key", "value")
			},
		},
		{
			name: "unset",
			call: func() error {
				return svc.ProjectConfigUnset(project.ID, "unknown.key")
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.call()
			if err == nil {
				t.Fatalf("%s error = nil, want project_config_key_invalid", tc.name)
			}
			runtimeErr, ok := err.(RuntimeError)
			if !ok || runtimeErr.Code != "project_config_key_invalid" {
				t.Fatalf("%s err = %#v, want RuntimeError(project_config_key_invalid)", tc.name, err)
			}
		})
	}
}

func TestProjectConfigAuditEntriesIncludeProjectID(t *testing.T) {
	store := newTestStore(t)
	svc := newTestServiceWithRuntime(t, store, 100, "local", "local")
	project, err := svc.AddProject(AddProjectInput{Slug: "api", Name: "API"})
	if err != nil {
		t.Fatalf("AddProject() error = %v", err)
	}
	if err := svc.ProjectConfigSet(project.ID, "agent.background", "Background"); err != nil {
		t.Fatalf("ProjectConfigSet() error = %v", err)
	}
	if err := svc.ProjectConfigUnset(project.Slug, "agent.background"); err != nil {
		t.Fatalf("ProjectConfigUnset() error = %v", err)
	}

	workspaceID := svc.Runtime().WorkspaceID
	rows, err := storage.NewAuditRepository(store.DB()).List(storage.AuditListOptions{
		WorkspaceID: &workspaceID,
		ProjectID:   &project.ID,
		Limit:       10,
	})
	if err != nil {
		t.Fatalf("AuditRepository.List(project config) error = %v", err)
	}
	var actions []string
	for _, row := range rows {
		if row.ProjectID == nil || *row.ProjectID != project.ID {
			t.Fatalf("row.ProjectID = %#v, want %q", row.ProjectID, project.ID)
		}
		if row.Action == "project.config.set" || row.Action == "project.config.unset" {
			actions = append(actions, row.Action)
		}
	}
	if len(actions) != 2 {
		t.Fatalf("project config audit actions = %#v, want set+unset", actions)
	}
}

func TestProjectConfigSetRechecksArchivedProjectInsideAuditTransaction(t *testing.T) {
	store := newTestStore(t)
	svc := newTestServiceWithRuntime(t, store, 100, "local", "local")
	project, err := svc.AddProject(AddProjectInput{Slug: "api", Name: "API"})
	if err != nil {
		t.Fatalf("AddProject() error = %v", err)
	}

	archivedAt := int64(200)
	if err := store.DB().Model(&storage.Project{}).
		Where("id = ? AND workspace_id = ?", project.ID, svc.Runtime().WorkspaceID).
		Updates(map[string]any{
			"status":      string(storage.ProjectStatusArchived),
			"archived_at": &archivedAt,
			"modified_at": archivedAt,
		}).Error; err != nil {
		t.Fatalf("force archive project error = %v", err)
	}

	if err := svc.ProjectConfigSet(project.ID, "agent.background", "should fail"); err == nil {
		t.Fatal("ProjectConfigSet() error = nil, want project_archived")
	} else if runtimeErr, ok := err.(RuntimeError); !ok || runtimeErr.Code != "project_archived" {
		t.Fatalf("ProjectConfigSet() err = %#v, want RuntimeError(project_archived)", err)
	}

	if _, ok, err := svc.ProjectConfigGet(project.ID, "agent.background"); err != nil {
		t.Fatalf("ProjectConfigGet() error = %v", err)
	} else if ok {
		t.Fatal("ProjectConfigGet() found value after archived write attempt, want missing")
	}
}

type failingAuditRepo struct {
	listRows []storage.AuditLogEntry
}

func (f *failingAuditRepo) Append(storage.AuditLogEntry) error {
	return storage.ErrNotFound
}

func (f *failingAuditRepo) List(storage.AuditListOptions) ([]storage.AuditLogEntry, error) {
	return f.listRows, nil
}

func TestTaskWriteCreatesAuditInSameTransaction(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()

	created, err := svc.Add(AddInput{Description: "audit me"})
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	logs, err := svc.ListAudit(AuditListInput{Limit: 10})
	if err != nil {
		t.Fatalf("ListAudit() error = %v", err)
	}
	if len(logs) == 0 {
		t.Fatal("ListAudit() returned no rows")
	}
	if logs[0].Action != "task.add" || logs[0].TargetID != created.UUID {
		t.Fatalf("logs[0] = %#v", logs[0])
	}
}

func TestReadMethodsRequireTaskReadPermission(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()

	created, err := svc.Add(AddInput{Description: "read me"})
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	svc.runtime.Role = Role("invalid")

	cases := []struct {
		name string
		call func() error
	}{
		{name: "List", call: func() error {
			_, err := svc.List(ListInput{})
			return err
		}},
		{name: "List target", call: func() error {
			_, err := svc.List(ListInput{Target: &created.UUID})
			return err
		}},
		{name: "ListReport", call: func() error {
			_, err := svc.ListReport("next", ListInput{})
			return err
		}},
		{name: "Info", call: func() error {
			_, err := svc.Info(created.UUID)
			return err
		}},
		{name: "Export", call: func() error {
			_, err := svc.Export()
			return err
		}},
		{name: "RunReport", call: func() error {
			_, err := svc.RunReport(ReportInput{Name: "next"})
			return err
		}},
		{name: "ExplainUrgency", call: func() error {
			_, err := svc.ExplainUrgency(created.UUID)
			return err
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.call(); err == nil {
				t.Fatal("error = nil, want permission denied")
			}
		})
	}
}

func TestAuditFailureRollsBackTaskWrite(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()

	svc.auditRepo = &failingAuditRepo{}
	if _, err := svc.Add(AddInput{Description: "should rollback"}); err == nil {
		t.Fatal("Add() error = nil, want audit failure")
	}
	tasks, err := svc.List(ListInput{})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(tasks) != 0 {
		t.Fatalf("tasks = %#v, want rollback to leave no task", tasks)
	}
}

func TestModifyCreatesAuditEntry(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()

	created, err := svc.Add(AddInput{Description: "before"})
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	priority := "H"
	if err := svc.Modify(created.UUID, ModifyInput{Priority: &priority}); err != nil {
		t.Fatalf("Modify() error = %v", err)
	}

	logs, err := svc.ListAudit(AuditListInput{Limit: 10})
	if err != nil {
		t.Fatalf("ListAudit() error = %v", err)
	}
	if len(logs) < 2 {
		t.Fatalf("logs = %#v, want at least 2 entries", logs)
	}
	if logs[0].Action != "task.modify" || logs[0].TargetID != created.UUID {
		t.Fatalf("logs[0] = %#v", logs[0])
	}
}

func TestUseContextCreatesAuditEntry(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()

	if err := svc.DefineContext("work", "project:work"); err != nil {
		t.Fatalf("DefineContext() error = %v", err)
	}
	if err := svc.UseContext("work"); err != nil {
		t.Fatalf("UseContext() error = %v", err)
	}

	logs, err := svc.ListAudit(AuditListInput{Limit: 10})
	if err != nil {
		t.Fatalf("ListAudit() error = %v", err)
	}
	if len(logs) < 2 {
		t.Fatalf("logs = %#v, want at least 2 entries", logs)
	}
	if logs[0].Action != "context.use" || logs[0].TargetType != "context" || logs[0].TargetID != "work" {
		t.Fatalf("logs[0] = %#v", logs[0])
	}
}

func TestUDAConfigWriteCreatesAuditEntry(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()

	if err := svc.SetConfig("uda.estimate.type", "numeric"); err != nil {
		t.Fatalf("SetConfig() error = %v", err)
	}

	logs, err := svc.ListAudit(AuditListInput{Limit: 10})
	if err != nil {
		t.Fatalf("ListAudit() error = %v", err)
	}
	if len(logs) == 0 {
		t.Fatal("ListAudit() returned no rows")
	}
	if logs[0].Action != "uda.schema.set" || logs[0].TargetType != "uda" || logs[0].TargetID != "estimate" {
		t.Fatalf("logs[0] = %#v", logs[0])
	}
}

func TestImportCreatesAuditEntry(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()

	count, err := svc.Import([]task.JSONTask{{
		Description: "imported task",
		Entry:       "1970-01-01T00:01:40Z",
		Modified:    "1970-01-01T00:01:40Z",
	}})
	if err != nil {
		t.Fatalf("Import() error = %v", err)
	}
	if count != 1 {
		t.Fatalf("Import() count = %d, want 1", count)
	}

	logs, err := svc.ListAudit(AuditListInput{Limit: 10})
	if err != nil {
		t.Fatalf("ListAudit() error = %v", err)
	}
	if len(logs) == 0 {
		t.Fatal("ListAudit() returned no rows")
	}
	if logs[0].Action != "task.import" || logs[0].TargetType != "task" {
		t.Fatalf("logs[0] = %#v", logs[0])
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(logs[0].PayloadJSON), &payload); err != nil {
		t.Fatalf("payload json = %q, err = %v", logs[0].PayloadJSON, err)
	}
	if got := payload["count"]; got != float64(1) {
		t.Fatalf("payload[count] = %#v, want 1", got)
	}
}

func TestServiceImportNormalizesProjectOnCreate(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()

	project, err := svc.AddProject(AddProjectInput{
		Slug: "work-project",
		Name: "Work Project",
	})
	if err != nil {
		t.Fatalf("AddProject() error = %v", err)
	}

	count, err := svc.Import([]task.JSONTask{{
		UUID:        "import-project-create",
		Description: "imported task",
		Status:      task.StatusPending,
		Entry:       "1970-01-01T00:01:40Z",
		Modified:    "1970-01-01T00:01:40Z",
		Project:     strptr("  " + strings.ToUpper(project.Slug) + "  "),
	}})
	if err != nil {
		t.Fatalf("Import() error = %v", err)
	}
	if count != 1 {
		t.Fatalf("Import() count = %d, want 1", count)
	}

	got, err := svc.ResolveTarget("import-project-create")
	if err != nil {
		t.Fatalf("ResolveTarget() error = %v", err)
	}
	if got.Project == nil || *got.Project != project.Slug {
		t.Fatalf("Project = %#v, want %q", got.Project, project.Slug)
	}
	if got.ProjectID == nil || *got.ProjectID != project.ID {
		t.Fatalf("ProjectID = %#v, want %q", got.ProjectID, project.ID)
	}
}

func TestServiceImportAssigneesFromObjectArray(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()

	count, err := svc.Import([]task.JSONTask{{
		UUID:        "import-assignee-object",
		Description: "imported task",
		Status:      task.StatusPending,
		Entry:       "1970-01-01T00:01:40Z",
		Modified:    "1970-01-01T00:01:40Z",
		Assignees:   []task.JSONAssignee{{Name: "local"}},
	}})
	if err != nil {
		t.Fatalf("Import() error = %v", err)
	}
	if count != 1 {
		t.Fatalf("Import() count = %d, want 1", count)
	}

	got, err := svc.ResolveTarget("import-assignee-object")
	if err != nil {
		t.Fatalf("ResolveTarget() error = %v", err)
	}
	if len(got.Assignees) != 1 || got.Assignees[0].Name != "local" {
		t.Fatalf("Assignees = %#v, want one local assignee", got.Assignees)
	}
}

func TestServiceImportAssigneesFromStringArray(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()

	count, err := svc.Import([]task.JSONTask{{
		UUID:        "import-assignee-string",
		Description: "imported task",
		Status:      task.StatusPending,
		Entry:       "1970-01-01T00:01:40Z",
		Modified:    "1970-01-01T00:01:40Z",
		Assignees:   []task.JSONAssignee{{UserID: "local"}},
	}})
	if err != nil {
		t.Fatalf("Import() error = %v", err)
	}
	if count != 1 {
		t.Fatalf("Import() count = %d, want 1", count)
	}

	got, err := svc.ResolveTarget("import-assignee-string")
	if err != nil {
		t.Fatalf("ResolveTarget() error = %v", err)
	}
	if len(got.Assignees) != 1 || got.Assignees[0].Name != "local" {
		t.Fatalf("Assignees = %#v, want one local assignee", got.Assignees)
	}
}

func TestServiceImportClearsAssigneesWithExplicitEmptyArray(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()

	created, err := svc.Add(AddInput{Description: "assigned task", Assignees: []string{"local"}})
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	count, err := svc.Import([]task.JSONTask{{
		UUID:      created.UUID,
		Entry:     "1970-01-01T00:01:40Z",
		Modified:  "1970-01-01T00:01:40Z",
		Assignees: []task.JSONAssignee{},
	}})
	if err != nil {
		t.Fatalf("Import(clear assignees) error = %v", err)
	}
	if count != 1 {
		t.Fatalf("Import() count = %d, want 1", count)
	}

	got, err := svc.ResolveTarget(created.UUID)
	if err != nil {
		t.Fatalf("ResolveTarget() error = %v", err)
	}
	if len(got.Assignees) != 0 {
		t.Fatalf("Assignees = %#v, want cleared assignees", got.Assignees)
	}
}

func TestServiceExportIncludesAssignees(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()

	created, err := svc.Add(AddInput{Description: "assigned task", Assignees: []string{"local"}})
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	rows, err := svc.Export()
	if err != nil {
		t.Fatalf("Export() error = %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("Export() len = %d, want 1", len(rows))
	}
	if rows[0].UUID != created.UUID {
		t.Fatalf("Exported UUID = %q, want %q", rows[0].UUID, created.UUID)
	}
	if len(rows[0].Assignees) != 1 || rows[0].Assignees[0].Name != "local" {
		t.Fatalf("Exported assignees = %#v, want one local assignee", rows[0].Assignees)
	}
}

func TestServiceImportNormalizesProjectOnUpdate(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()

	projectA, err := svc.AddProject(AddProjectInput{Slug: "alpha", Name: "Alpha"})
	if err != nil {
		t.Fatalf("AddProject(alpha) error = %v", err)
	}
	projectB, err := svc.AddProject(AddProjectInput{Slug: "beta", Name: "Beta"})
	if err != nil {
		t.Fatalf("AddProject(beta) error = %v", err)
	}

	if _, err := svc.Import([]task.JSONTask{{
		UUID:        "import-project-update",
		Description: "imported task",
		Status:      task.StatusPending,
		Entry:       "1970-01-01T00:01:40Z",
		Modified:    "1970-01-01T00:01:40Z",
		Project:     &projectA.Slug,
	}}); err != nil {
		t.Fatalf("Import(create) error = %v", err)
	}

	if _, err := svc.Import([]task.JSONTask{{
		UUID:        "import-project-update",
		Description: "imported task",
		Status:      task.StatusPending,
		Entry:       "1970-01-01T00:01:40Z",
		Modified:    "1970-01-01T00:01:40Z",
		Project:     strptr(" " + strings.ToUpper(projectB.Slug) + " "),
	}}); err != nil {
		t.Fatalf("Import(update) error = %v", err)
	}

	got, err := svc.ResolveTarget("import-project-update")
	if err != nil {
		t.Fatalf("ResolveTarget() error = %v", err)
	}
	if got.Project == nil || *got.Project != projectB.Slug {
		t.Fatalf("Project = %#v, want %q", got.Project, projectB.Slug)
	}
	if got.ProjectID == nil || *got.ProjectID != projectB.ID {
		t.Fatalf("ProjectID = %#v, want %q", got.ProjectID, projectB.ID)
	}
}

func TestImportRejectsUnregisteredProjectAtomically(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()

	validProject, err := svc.AddProject(AddProjectInput{Slug: "known", Name: "Known"})
	if err != nil {
		t.Fatalf("AddProject(known) error = %v", err)
	}

	count, err := svc.Import([]task.JSONTask{
		{
			UUID:        "import-known",
			Description: "known project task",
			Status:      task.StatusPending,
			Entry:       "1970-01-01T00:01:40Z",
			Modified:    "1970-01-01T00:01:40Z",
			Project:     &validProject.Slug,
		},
		{
			UUID:        "import-missing",
			Description: "missing project task",
			Status:      task.StatusPending,
			Entry:       "1970-01-01T00:01:41Z",
			Modified:    "1970-01-01T00:01:41Z",
			Project:     strptr("missing"),
		},
	})
	if err == nil {
		t.Fatal("Import(unregistered project) error = nil, want project_not_found")
	}
	if runtimeErr, ok := err.(RuntimeError); !ok || runtimeErr.Code != "project_not_found" {
		t.Fatalf("Import(unregistered project) err = %#v, want RuntimeError(project_not_found)", err)
	}
	if count != 0 {
		t.Fatalf("Import(unregistered project) count = %d, want 0", count)
	}
	if tasks, err := svc.Export(); err != nil {
		t.Fatalf("Export() error = %v", err)
	} else if len(tasks) != 0 {
		t.Fatalf("Export() tasks = %#v, want atomic rollback", tasks)
	}
}

func TestImportAllowsArchivedProjectRoundTripOnlyForExistingBinding(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()

	project, err := svc.AddProject(AddProjectInput{Slug: "legacy", Name: "Legacy"})
	if err != nil {
		t.Fatalf("AddProject(legacy) error = %v", err)
	}
	count, err := svc.Import([]task.JSONTask{{
		UUID:        "legacy-task",
		Description: "legacy task",
		Status:      task.StatusPending,
		Entry:       "1970-01-01T00:01:40Z",
		Modified:    "1970-01-01T00:01:40Z",
		Project:     &project.Slug,
	}})
	if err != nil || count != 1 {
		t.Fatalf("Import(initial legacy task) = (%d, %v), want (1, nil)", count, err)
	}
	if _, err := svc.ArchiveProject(project.ID); err != nil {
		t.Fatalf("ArchiveProject(legacy) error = %v", err)
	}

	if _, err := svc.Import([]task.JSONTask{{
		UUID:        "legacy-task",
		Description: "legacy task updated",
		Status:      task.StatusPending,
		Entry:       "1970-01-01T00:01:40Z",
		Modified:    "1970-01-01T00:01:42Z",
		Project:     &project.Slug,
	}}); err != nil {
		t.Fatalf("Import(round-trip archived project) error = %v", err)
	}

	got, err := svc.ResolveTarget("legacy-task")
	if err != nil {
		t.Fatalf("ResolveTarget(legacy-task) error = %v", err)
	}
	if got.Project == nil || *got.Project != project.Slug || got.ProjectID == nil || *got.ProjectID != project.ID {
		t.Fatalf("legacy task project binding = (%#v, %#v), want (%q, %q)", got.Project, got.ProjectID, project.Slug, project.ID)
	}
	if got.Description != "legacy task updated" {
		t.Fatalf("legacy task description = %q, want updated", got.Description)
	}

	if _, err := svc.Import([]task.JSONTask{{
		UUID:        "new-archived-task",
		Description: "new archived task",
		Status:      task.StatusPending,
		Entry:       "1970-01-01T00:01:45Z",
		Modified:    "1970-01-01T00:01:45Z",
		Project:     &project.Slug,
	}}); err == nil {
		t.Fatal("Import(new archived project task) error = nil, want project_archived")
	} else if runtimeErr, ok := err.(RuntimeError); !ok || runtimeErr.Code != "project_archived" {
		t.Fatalf("Import(new archived project task) err = %#v, want RuntimeError(project_archived)", err)
	}
}

func TestExportRejectsProjectInvariantViolation(t *testing.T) {
	store := newTestStore(t)
	svc := newTestServiceWithRuntime(t, store, 100, "local", "local")
	project, err := svc.AddProject(AddProjectInput{Slug: "api", Name: "API"})
	if err != nil {
		t.Fatalf("AddProject(api) error = %v", err)
	}
	created, err := svc.Add(AddInput{Description: "broken export task", Project: &project.Slug})
	if err != nil {
		t.Fatalf("Add(task) error = %v", err)
	}

	if err := store.DB().Model(&storage.Task{}).
		Where("uuid = ? AND workspace_id = ?", created.UUID, svc.Runtime().WorkspaceID).
		Update("project", "mismatch").Error; err != nil {
		t.Fatalf("corrupt project slug error = %v", err)
	}

	if _, err := svc.Export(); err == nil {
		t.Fatal("Export() error = nil, want project_invariant_violation")
	} else if runtimeErr, ok := err.(RuntimeError); !ok || runtimeErr.Code != "project_invariant_violation" {
		t.Fatalf("Export() err = %#v, want RuntimeError(project_invariant_violation)", err)
	}
}

func TestRecurringChildOnArchivedProjectWritesAuditWarning(t *testing.T) {
	store := newTestStore(t)
	svc := newTestServiceWithRuntime(t, store, mustUnix(t, "2030-01-01T10:00:00Z"), "local", "local")
	project, err := svc.AddProject(AddProjectInput{Slug: "legacy", Name: "Legacy"})
	if err != nil {
		t.Fatalf("AddProject(legacy) error = %v", err)
	}
	due := mustUnix(t, "2030-01-01T23:59:59Z")
	until := mustUnix(t, "2030-01-03T23:59:59Z")
	recur := "daily"
	parent, err := svc.Add(AddInput{
		Description: "legacy recurring task",
		Project:     &project.Slug,
		Due:         &due,
		Until:       &until,
		Recur:       &recur,
	})
	if err != nil {
		t.Fatalf("Add(recurring) error = %v", err)
	}
	if _, err := svc.ArchiveProject(project.ID); err != nil {
		t.Fatalf("ArchiveProject(legacy) error = %v", err)
	}
	children, err := svc.List(ListInput{})
	if err != nil || len(children) != 1 {
		t.Fatalf("List(children before done) = (%#v, %v), want one child", children, err)
	}
	firstChild := children[0]
	svc.clock = FixedClock{NowUnix: mustUnix(t, "2030-01-02T10:00:00Z")}
	if err := svc.Done(firstChild.UUID); err != nil {
		t.Fatalf("Done(first child) error = %v", err)
	}

	tasks, err := svc.List(ListInput{})
	if err != nil {
		t.Fatalf("List() after recurrence error = %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("List() after recurrence = %#v, want exactly next child", tasks)
	}
	nextChild := tasks[0]
	if nextChild.Project == nil || *nextChild.Project != project.Slug || nextChild.ProjectID == nil || *nextChild.ProjectID != project.ID {
		t.Fatalf("next child project binding = (%#v, %#v), want (%q, %q)", nextChild.Project, nextChild.ProjectID, project.Slug, project.ID)
	}

	logs, err := svc.ListAudit(AuditListInput{Limit: 20})
	if err != nil {
		t.Fatalf("ListAudit() error = %v", err)
	}
	found := false
	for _, log := range logs {
		if log.Action != "task.recurrence.archived_project" {
			continue
		}
		found = true
		var payload map[string]any
		if err := json.Unmarshal([]byte(log.PayloadJSON), &payload); err != nil {
			t.Fatalf("recurrence warning payload json = %q, err = %v", log.PayloadJSON, err)
		}
		if log.TargetID != nextChild.UUID {
			t.Fatalf("warning TargetID = %q, want child %q", log.TargetID, nextChild.UUID)
		}
		if log.ProjectID == nil || *log.ProjectID != project.ID {
			t.Fatalf("warning ProjectID = %#v, want %q", log.ProjectID, project.ID)
		}
		if payload["parent_uuid"] != parent.UUID || payload["child_uuid"] != nextChild.UUID || payload["project_id"] != project.ID || payload["project_slug"] != project.Slug {
			t.Fatalf("warning payload = %#v", payload)
		}
	}
	if !found {
		t.Fatal("missing task.recurrence.archived_project audit warning")
	}
}

func TestAddTaskRequiresActiveProject(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()

	archived, err := svc.AddProject(AddProjectInput{Slug: "legacy", Name: "Legacy"})
	if err != nil {
		t.Fatalf("AddProject(legacy) error = %v", err)
	}
	if _, err := svc.ArchiveProject(archived.ID); err != nil {
		t.Fatalf("ArchiveProject(legacy) error = %v", err)
	}
	active, err := svc.AddProject(AddProjectInput{Slug: "api", Name: "API"})
	if err != nil {
		t.Fatalf("AddProject(api) error = %v", err)
	}

	if _, err := svc.Add(AddInput{Description: "missing project", Project: strptr("missing")}); err == nil {
		t.Fatal("Add(missing project) error = nil, want project_not_found")
	} else if runtimeErr, ok := err.(RuntimeError); !ok || runtimeErr.Code != "project_not_found" {
		t.Fatalf("Add(missing project) err = %#v, want RuntimeError(project_not_found)", err)
	}

	if _, err := svc.Add(AddInput{Description: "archived project", Project: &archived.Slug}); err == nil {
		t.Fatal("Add(archived project) error = nil, want project_archived")
	} else if runtimeErr, ok := err.(RuntimeError); !ok || runtimeErr.Code != "project_archived" {
		t.Fatalf("Add(archived project) err = %#v, want RuntimeError(project_archived)", err)
	}

	created, err := svc.Add(AddInput{Description: "active project", Project: strptr("  " + strings.ToUpper(active.Slug) + "  ")})
	if err != nil {
		t.Fatalf("Add(active project) error = %v", err)
	}
	if created.Project == nil || *created.Project != active.Slug {
		t.Fatalf("created.Project = %#v, want %q", created.Project, active.Slug)
	}
	if created.ProjectID == nil || *created.ProjectID != active.ID {
		t.Fatalf("created.ProjectID = %#v, want %q", created.ProjectID, active.ID)
	}
}

func TestModifyTaskProjectClearsAndRejectsArchivedAssignment(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()

	current, err := svc.AddProject(AddProjectInput{Slug: "current", Name: "Current"})
	if err != nil {
		t.Fatalf("AddProject(current) error = %v", err)
	}
	otherArchived, err := svc.AddProject(AddProjectInput{Slug: "archived", Name: "Archived"})
	if err != nil {
		t.Fatalf("AddProject(archived) error = %v", err)
	}
	active, err := svc.AddProject(AddProjectInput{Slug: "next", Name: "Next"})
	if err != nil {
		t.Fatalf("AddProject(next) error = %v", err)
	}
	clearTask, err := svc.Add(AddInput{Description: "clear me", Project: &current.Slug})
	if err != nil {
		t.Fatalf("Add(clear task) error = %v", err)
	}
	moveTask, err := svc.Add(AddInput{Description: "move me", Project: &current.Slug})
	if err != nil {
		t.Fatalf("Add(move task) error = %v", err)
	}
	rejectTask, err := svc.Add(AddInput{Description: "reject me", Project: &current.Slug})
	if err != nil {
		t.Fatalf("Add(reject task) error = %v", err)
	}

	if _, err := svc.ArchiveProject(current.ID); err != nil {
		t.Fatalf("ArchiveProject(current) error = %v", err)
	}
	if _, err := svc.ArchiveProject(otherArchived.ID); err != nil {
		t.Fatalf("ArchiveProject(otherArchived) error = %v", err)
	}

	if err := svc.Modify(clearTask.UUID, ModifyInput{Project: strptr(" ")}); err != nil {
		t.Fatalf("Modify(clear project) error = %v", err)
	}
	cleared, err := svc.ResolveTarget(clearTask.UUID)
	if err != nil {
		t.Fatalf("ResolveTarget(clearTask) error = %v", err)
	}
	if cleared.Project != nil || cleared.ProjectID != nil {
		t.Fatalf("cleared task project fields = (%#v, %#v), want nil,nil", cleared.Project, cleared.ProjectID)
	}

	if err := svc.Modify(moveTask.UUID, ModifyInput{Project: &active.Slug}); err != nil {
		t.Fatalf("Modify(move to active) error = %v", err)
	}
	moved, err := svc.ResolveTarget(moveTask.UUID)
	if err != nil {
		t.Fatalf("ResolveTarget(moveTask) error = %v", err)
	}
	if moved.Project == nil || *moved.Project != active.Slug {
		t.Fatalf("moved.Project = %#v, want %q", moved.Project, active.Slug)
	}
	if moved.ProjectID == nil || *moved.ProjectID != active.ID {
		t.Fatalf("moved.ProjectID = %#v, want %q", moved.ProjectID, active.ID)
	}

	if err := svc.Modify(rejectTask.UUID, ModifyInput{Project: &otherArchived.Slug}); err == nil {
		t.Fatal("Modify(assign archived project) error = nil, want project_archived")
	} else if runtimeErr, ok := err.(RuntimeError); !ok || runtimeErr.Code != "project_archived" {
		t.Fatalf("Modify(assign archived project) err = %#v, want RuntimeError(project_archived)", err)
	}
}

func TestTaskAuditEntriesTrackProjectChanges(t *testing.T) {
	store := newTestStore(t)
	svc := newTestServiceWithRuntime(t, store, 100, "local", "local")

	projectA, err := svc.AddProject(AddProjectInput{Slug: "alpha", Name: "Alpha"})
	if err != nil {
		t.Fatalf("AddProject(alpha) error = %v", err)
	}
	projectB, err := svc.AddProject(AddProjectInput{Slug: "beta", Name: "Beta"})
	if err != nil {
		t.Fatalf("AddProject(beta) error = %v", err)
	}
	created, err := svc.Add(AddInput{Description: "audited task", Project: &projectA.Slug})
	if err != nil {
		t.Fatalf("Add(task) error = %v", err)
	}
	if err := svc.Modify(created.UUID, ModifyInput{Project: &projectB.Slug}); err != nil {
		t.Fatalf("Modify(move to beta) error = %v", err)
	}
	if err := svc.Modify(created.UUID, ModifyInput{Project: strptr("")}); err != nil {
		t.Fatalf("Modify(clear project) error = %v", err)
	}
	if err := svc.Modify(created.UUID, ModifyInput{Project: &projectA.Slug}); err != nil {
		t.Fatalf("Modify(rebind alpha) error = %v", err)
	}

	workspaceID := svc.Runtime().WorkspaceID
	rows, err := storage.NewAuditRepository(store.DB()).List(storage.AuditListOptions{
		WorkspaceID: &workspaceID,
		Limit:       10,
	})
	if err != nil {
		t.Fatalf("AuditRepository.List() error = %v", err)
	}
	if len(rows) < 4 {
		t.Fatalf("audit rows = %#v, want at least 4 task rows", rows)
	}

	assertProjectAudit := func(row storage.AuditLogEntry, wantAction string, wantProjectID *string, beforeID, beforeSlug, afterID, afterSlug *string) {
		t.Helper()
		if row.Action != wantAction {
			t.Fatalf("row.Action = %q, want %q", row.Action, wantAction)
		}
		if row.ProjectID == nil && wantProjectID != nil || row.ProjectID != nil && wantProjectID == nil {
			t.Fatalf("row.ProjectID = %#v, want %#v", row.ProjectID, wantProjectID)
		}
		if row.ProjectID != nil && wantProjectID != nil && *row.ProjectID != *wantProjectID {
			t.Fatalf("row.ProjectID = %q, want %q", *row.ProjectID, *wantProjectID)
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(row.PayloadJSON), &payload); err != nil {
			t.Fatalf("payload json = %q, err = %v", row.PayloadJSON, err)
		}
		assertPayloadString := func(key string, want *string) {
			t.Helper()
			got, ok := payload[key]
			if want == nil {
				if ok && got != nil {
					t.Fatalf("payload[%s] = %#v, want nil/absent", key, got)
				}
				return
			}
			if !ok {
				t.Fatalf("payload missing key %q: %#v", key, payload)
			}
			gotStr, ok := got.(string)
			if !ok || gotStr != *want {
				t.Fatalf("payload[%s] = %#v, want %q", key, got, *want)
			}
		}
		assertPayloadString("before_project_id", beforeID)
		assertPayloadString("before_project_slug", beforeSlug)
		assertPayloadString("after_project_id", afterID)
		assertPayloadString("after_project_slug", afterSlug)
	}

	assertProjectAudit(rows[0], "task.modify", &projectA.ID, nil, nil, &projectA.ID, &projectA.Slug)
	assertProjectAudit(rows[1], "task.modify", &projectB.ID, &projectB.ID, &projectB.Slug, nil, nil)
	assertProjectAudit(rows[2], "task.modify", &projectA.ID, &projectA.ID, &projectA.Slug, &projectB.ID, &projectB.Slug)
	assertProjectAudit(rows[3], "task.add", &projectA.ID, nil, nil, &projectA.ID, &projectA.Slug)
}

func TestReplaceEditableTaskNormalizesProjectAndIgnoresForgedProjectID(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()

	project, err := svc.AddProject(AddProjectInput{Slug: "work", Name: "Work"})
	if err != nil {
		t.Fatalf("AddProject() error = %v", err)
	}
	created, err := svc.Add(AddInput{Description: "editable task"})
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	edited, err := svc.ResolveTarget(created.UUID)
	if err != nil {
		t.Fatalf("ResolveTarget() error = %v", err)
	}
	edited.Project = strptr(" " + strings.ToUpper(project.Slug) + " ")
	edited.ProjectID = strptr("forged-project-id")

	if err := svc.ReplaceEditableTask(created.UUID, edited); err != nil {
		t.Fatalf("ReplaceEditableTask() error = %v", err)
	}

	got, err := svc.ResolveTarget(created.UUID)
	if err != nil {
		t.Fatalf("ResolveTarget() error = %v", err)
	}
	if got.Project == nil || *got.Project != project.Slug {
		t.Fatalf("Project = %#v, want %q", got.Project, project.Slug)
	}
	if got.ProjectID == nil || *got.ProjectID != project.ID {
		t.Fatalf("ProjectID = %#v, want %q", got.ProjectID, project.ID)
	}
}

func TestReplaceEditableTaskClearsProjectFields(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()

	project, err := svc.AddProject(AddProjectInput{Slug: "work", Name: "Work"})
	if err != nil {
		t.Fatalf("AddProject() error = %v", err)
	}
	created, err := svc.Add(AddInput{Description: "editable task", Project: &project.Slug})
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	edited, err := svc.ResolveTarget(created.UUID)
	if err != nil {
		t.Fatalf("ResolveTarget() error = %v", err)
	}
	edited.Project = strptr(" ")
	edited.ProjectID = strptr("forged-project-id")

	if err := svc.ReplaceEditableTask(created.UUID, edited); err != nil {
		t.Fatalf("ReplaceEditableTask() error = %v", err)
	}

	got, err := svc.ResolveTarget(created.UUID)
	if err != nil {
		t.Fatalf("ResolveTarget() error = %v", err)
	}
	if got.Project != nil {
		t.Fatalf("Project = %#v, want nil", got.Project)
	}
	if got.ProjectID != nil {
		t.Fatalf("ProjectID = %#v, want nil", got.ProjectID)
	}
}

func TestReplaceEditableTaskAuditUsesOriginalProjectAsBefore(t *testing.T) {
	store := newTestStore(t)
	svc := newTestServiceWithRuntime(t, store, 100, "local", "local")

	projectA, err := svc.AddProject(AddProjectInput{Slug: "alpha", Name: "Alpha"})
	if err != nil {
		t.Fatalf("AddProject(alpha) error = %v", err)
	}
	projectB, err := svc.AddProject(AddProjectInput{Slug: "beta", Name: "Beta"})
	if err != nil {
		t.Fatalf("AddProject(beta) error = %v", err)
	}
	created, err := svc.Add(AddInput{Description: "editable task", Project: &projectA.Slug})
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	edited, err := svc.ResolveTarget(created.UUID)
	if err != nil {
		t.Fatalf("ResolveTarget() error = %v", err)
	}
	edited.Project = &projectB.Slug
	edited.ProjectID = strptr("forged-project-id")

	if err := svc.ReplaceEditableTask(created.UUID, edited); err != nil {
		t.Fatalf("ReplaceEditableTask() error = %v", err)
	}

	workspaceID := svc.Runtime().WorkspaceID
	rows, err := storage.NewAuditRepository(store.DB()).List(storage.AuditListOptions{
		WorkspaceID: &workspaceID,
		Limit:       5,
	})
	if err != nil {
		t.Fatalf("AuditRepository.List() error = %v", err)
	}
	if len(rows) == 0 || rows[0].Action != "task.edit" {
		t.Fatalf("latest audit row = %#v, want task.edit", rows)
	}
	if rows[0].ProjectID == nil || *rows[0].ProjectID != projectA.ID {
		t.Fatalf("task.edit ProjectID = %#v, want original project %q", rows[0].ProjectID, projectA.ID)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(rows[0].PayloadJSON), &payload); err != nil {
		t.Fatalf("payload json = %q, err = %v", rows[0].PayloadJSON, err)
	}
	if payload["before_project_id"] != projectA.ID || payload["after_project_id"] != projectB.ID {
		t.Fatalf("task.edit payload = %#v, want before alpha and after beta", payload)
	}
}

func TestAuditFailureRollsBackContextWrite(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()

	svc.auditRepo = &failingAuditRepo{}
	if err := svc.DefineContext("work", "project:work"); err == nil {
		t.Fatal("DefineContext() error = nil, want audit failure")
	}

	contexts, err := svc.ContextList()
	if err != nil {
		t.Fatalf("ContextList() error = %v", err)
	}
	if len(contexts) != 0 {
		t.Fatalf("contexts = %#v, want rollback to leave no context", contexts)
	}
}

func TestServicePersistsUDAValues(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()

	if err := svc.DefineUDA("estimate", "numeric", "Estimate", []string{"1", "2", "3", "5"}, ""); err != nil {
		t.Fatalf("DefineUDA() error = %v", err)
	}
	created, err := svc.Add(AddInput{Description: "task", UDAs: map[string]string{"estimate": "3"}})
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	got, err := svc.ResolveTarget(created.UUID)
	if err != nil {
		t.Fatalf("ResolveTarget() error = %v", err)
	}
	if got.UDAs["estimate"].Raw != "3" || got.UDAs["estimate"].Type != "numeric" {
		t.Fatalf("UDA after add = %#v", got.UDAs)
	}
	if err := svc.Modify(created.UUID, ModifyInput{UDAs: map[string]string{"estimate": "5"}}); err != nil {
		t.Fatalf("Modify(set UDA) error = %v", err)
	}
	got, _ = svc.ResolveTarget(created.UUID)
	if got.UDAs["estimate"].Raw != "5" {
		t.Fatalf("UDA after modify = %#v", got.UDAs)
	}
	if err := svc.Modify(created.UUID, ModifyInput{ClearUDAs: []string{"estimate"}}); err != nil {
		t.Fatalf("Modify(clear UDA) error = %v", err)
	}
	got, _ = svc.ResolveTarget(created.UUID)
	if _, ok := got.UDAs["estimate"]; ok {
		t.Fatalf("UDA after clear = %#v, want absent", got.UDAs)
	}
}

func TestConfigSetRoutesUDASchemaKeys(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()

	if err := svc.SetConfig("uda.estimate.type", "numeric"); err != nil {
		t.Fatalf("SetConfig(type) error = %v", err)
	}
	if err := svc.SetConfig("uda.estimate.label", "Estimate"); err != nil {
		t.Fatalf("SetConfig(label) error = %v", err)
	}
	if err := svc.SetConfig("uda.estimate.values", "1,2,3,5,8"); err != nil {
		t.Fatalf("SetConfig(values) error = %v", err)
	}
	got, ok, err := svc.GetConfig("uda.estimate.values")
	if err != nil {
		t.Fatalf("GetConfig(values) error = %v", err)
	}
	if !ok || got != "1,2,3,5,8" {
		t.Fatalf("uda.estimate.values = %q, %v", got, ok)
	}
	list, err := svc.ConfigValues()
	if err != nil {
		t.Fatalf("ConfigValues() error = %v", err)
	}
	if list["uda.estimate.type"] != "numeric" || list["uda.estimate.label"] != "Estimate" || list["uda.estimate.values"] != "1,2,3,5,8" {
		t.Fatalf("ConfigValues() = %#v", list)
	}
	if err := svc.UnsetConfig("uda.estimate.values"); err != nil {
		t.Fatalf("UnsetConfig(values) error = %v", err)
	}
	got, ok, err = svc.GetConfig("uda.estimate.values")
	if err != nil {
		t.Fatalf("GetConfig(values after unset) error = %v", err)
	}
	if ok || got != "" {
		t.Fatalf("uda.estimate.values after unset = %q, %v", got, ok)
	}
}

func TestConfigSetOverridesRuntimeUDADefaults(t *testing.T) {
	store, err := storage.Open(filepath.Join(t.TempDir(), "taskg.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	svc, err := NewService(ServiceOptions{
		Store:         store,
		Clock:         FixedClock{NowUnix: 100},
		RuntimeConfig: map[string]string{"uda.estimate.type": "string"},
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	if err := svc.SetConfig("uda.estimate.type", "numeric"); err != nil {
		t.Fatalf("SetConfig(type) error = %v", err)
	}
	got, ok, err := svc.GetConfig("uda.estimate.type")
	if err != nil {
		t.Fatalf("GetConfig(type) error = %v", err)
	}
	if !ok || got != "numeric" {
		t.Fatalf("uda.estimate.type = %q, %v; want numeric from DB", got, ok)
	}
	values, err := svc.ConfigValues()
	if err != nil {
		t.Fatalf("ConfigValues() error = %v", err)
	}
	if values["uda.estimate.type"] != "numeric" {
		t.Fatalf("ConfigValues()[uda.estimate.type] = %q, want numeric from DB", values["uda.estimate.type"])
	}
	if _, err := svc.Add(AddInput{Description: "bad estimate", UDAs: map[string]string{"estimate": "not-number"}}); err == nil {
		t.Fatal("Add() error = nil, want DB numeric schema to override runtime string default")
	}
}

func TestConfigSetOverridesRuntimeMetaDefaults(t *testing.T) {
	store, err := storage.Open(filepath.Join(t.TempDir(), "taskg.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	svc, err := NewService(ServiceOptions{
		Store: store,
		Clock: FixedClock{NowUnix: 100},
		RuntimeConfig: map[string]string{
			"date.format":                        "epoch",
			"urgency.uda.estimate.coefficient":   "2",
			"urgency.uda.estimate.3.coefficient": "3",
		},
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	if err := svc.SetConfig("date.format", "rfc3339"); err != nil {
		t.Fatalf("SetConfig(date.format) error = %v", err)
	}
	if err := svc.SetConfig("urgency.uda.estimate.coefficient", "10"); err != nil {
		t.Fatalf("SetConfig(urgency coefficient) error = %v", err)
	}
	got, ok, err := svc.GetConfig("date.format")
	if err != nil {
		t.Fatalf("GetConfig(date.format) error = %v", err)
	}
	if !ok || got != "rfc3339" {
		t.Fatalf("date.format = %q, %v; want rfc3339 from DB", got, ok)
	}
	got, ok, err = svc.GetConfig("urgency.uda.estimate.coefficient")
	if err != nil {
		t.Fatalf("GetConfig(urgency coefficient) error = %v", err)
	}
	if !ok || got != "10" {
		t.Fatalf("urgency coefficient = %q, %v; want 10 from DB", got, ok)
	}
}

func TestAddUserCreatesPersonalWorkspaceAndOwnerMembership(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()

	created, err := svc.AddUser(AddUserInput{Name: "alice", Email: "alice@example.test"})
	if err != nil {
		t.Fatalf("AddUser() error = %v", err)
	}
	if created.Name != "alice" {
		t.Fatalf("created.Name = %q, want alice", created.Name)
	}
	if created.Email == nil || *created.Email != "alice@example.test" {
		t.Fatalf("created.Email = %#v", created.Email)
	}
	if created.DefaultWorkspaceID == nil {
		t.Fatal("created.DefaultWorkspaceID = nil")
	}

	userRepo := storage.NewUserRepository(svc.store.DB())
	wsRepo := storage.NewWorkspaceRepository(svc.store.DB())
	memberRepo := storage.NewMemberRepository(svc.store.DB())

	user, err := userRepo.GetByName("alice")
	if err != nil {
		t.Fatalf("GetByName(alice) error = %v", err)
	}
	ws, err := wsRepo.GetBySlug("alice")
	if err != nil {
		t.Fatalf("GetBySlug(alice) error = %v", err)
	}
	if ws.Visibility != "private" {
		t.Fatalf("workspace visibility = %q, want private", ws.Visibility)
	}
	if user.DefaultWorkspaceID == nil || *user.DefaultWorkspaceID != ws.ID {
		t.Fatalf("user.DefaultWorkspaceID = %#v, want %q", user.DefaultWorkspaceID, ws.ID)
	}
	member, err := memberRepo.Get(user.ID, ws.ID)
	if err != nil {
		t.Fatalf("Get(membership) error = %v", err)
	}
	if member.Role != string(RoleOwner) {
		t.Fatalf("member.Role = %q, want owner", member.Role)
	}

	logs, err := svc.ListAudit(AuditListInput{Limit: 10})
	if err != nil {
		t.Fatalf("ListAudit() error = %v", err)
	}
	if len(logs) == 0 || logs[0].Action != "user.add" {
		t.Fatalf("local logs = %#v, want user.add", logs)
	}
	aliceSvc := newTestServiceWithRuntime(t, svc.store, 100, "alice", "alice")
	aliceLogs, err := aliceSvc.ListAudit(AuditListInput{Limit: 10})
	if err != nil {
		t.Fatalf("ListAudit(alice workspace) error = %v", err)
	}
	if len(aliceLogs) == 0 || aliceLogs[0].Action != "workspace.add" || aliceLogs[0].WorkspaceID == nil || *aliceLogs[0].WorkspaceID != ws.ID {
		t.Fatalf("alice logs = %#v, want workspace.add in personal workspace", aliceLogs)
	}
}

func TestAddUserRejectsInvalidPersonalWorkspaceSlug(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()

	if _, err := svc.AddUser(AddUserInput{Name: "Alice"}); err == nil {
		t.Fatal("AddUser(Alice) error = nil, want invalid workspace slug")
	}
	if _, err := storage.NewUserRepository(svc.store.DB()).GetByName("Alice"); err != storage.ErrNotFound {
		t.Fatalf("GetByName(Alice) error = %v, want ErrNotFound", err)
	}
}

func TestUseUserWritesActiveUserAndIgnoresWorkspaceOverride(t *testing.T) {
	store := newTestStore(t)
	userRepo := storage.NewUserRepository(store.DB())
	wsRepo := storage.NewWorkspaceRepository(store.DB())
	localUser, err := userRepo.GetByName("local")
	if err != nil {
		t.Fatalf("GetByName(local) error = %v", err)
	}
	work := mustCreateWorkspaceRecord(t, store, storage.Workspace{
		ID:              "ws-work",
		Slug:            "work",
		Name:            "Work",
		CreatedByUserID: &localUser.ID,
		Visibility:      "team",
		SettingsJSON:    "{}",
		CreatedAt:       100,
		ModifiedAt:      100,
	})
	mustUpsertMembershipRecord(t, store, storage.Membership{
		UserID:      localUser.ID,
		WorkspaceID: work.ID,
		Role:        string(RoleOwner),
		JoinedAt:    100,
		ModifiedAt:  100,
	})

	creator := newTestServiceWithRuntime(t, store, 100, "local", "local")
	alice, err := creator.AddUser(AddUserInput{Name: "alice"})
	if err != nil {
		t.Fatalf("AddUser(alice) error = %v", err)
	}

	svc := newTestServiceWithRuntime(t, store, 100, "local", "work")
	if err := svc.UseUser("alice"); err != nil {
		t.Fatalf("UseUser() error = %v", err)
	}
	activeUserID, ok, err := store.GetMeta("active_user_id")
	if err != nil {
		t.Fatalf("GetMeta(active_user_id) error = %v", err)
	}
	if !ok || activeUserID != alice.ID {
		t.Fatalf("active_user_id = %q, %v; want %q", activeUserID, ok, alice.ID)
	}
	if activeWorkspace, ok, err := store.GetMeta(activeWorkspaceMetaKey(alice.ID)); err != nil {
		t.Fatalf("GetMeta(active_workspace.%s) error = %v", alice.ID, err)
	} else if ok {
		t.Fatalf("alice active workspace = %q, want unset", activeWorkspace)
	}
	_, err = wsRepo.GetByID(*alice.DefaultWorkspaceID)
	if err != nil {
		t.Fatalf("GetByID(default workspace) error = %v", err)
	}
}

func TestAddWorkspaceCreatesOwnerMembershipAndUseWorkspaceWritesMeta(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()

	created, err := svc.AddWorkspace(AddWorkspaceInput{Slug: "work", Name: "Work", Visibility: "team"})
	if err != nil {
		t.Fatalf("AddWorkspace() error = %v", err)
	}
	if created.Slug != "work" || created.Visibility != "team" {
		t.Fatalf("created = %#v", created)
	}

	member, err := storage.NewMemberRepository(svc.store.DB()).Get(svc.Runtime().ActorUserID, created.ID)
	if err != nil {
		t.Fatalf("Get(owner membership) error = %v", err)
	}
	if member.Role != string(RoleOwner) {
		t.Fatalf("member.Role = %q, want owner", member.Role)
	}

	if err := svc.UseWorkspace("work"); err != nil {
		t.Fatalf("UseWorkspace() error = %v", err)
	}
	got, ok, err := svc.store.GetMeta(activeWorkspaceMetaKey(svc.Runtime().ActorUserID))
	if err != nil {
		t.Fatalf("GetMeta(active workspace) error = %v", err)
	}
	if !ok || got != created.ID {
		t.Fatalf("active workspace = %q, %v; want %q", got, ok, created.ID)
	}
}

func TestModifyWorkspaceWritesAuditForAdmin(t *testing.T) {
	store := newTestStore(t)
	ownerSvc := newTestServiceWithRuntime(t, store, 100, "local", "local")
	ws, err := ownerSvc.AddWorkspace(AddWorkspaceInput{Slug: "team", Name: "Team", Visibility: "team"})
	if err != nil {
		t.Fatalf("AddWorkspace() error = %v", err)
	}

	admin := mustCreateUserRecord(t, store, storage.User{ID: "user-admin-work", Name: "work-admin", CreatedAt: 100, ModifiedAt: 100})
	mustUpsertMembershipRecord(t, store, storage.Membership{
		UserID:      admin.ID,
		WorkspaceID: ws.ID,
		Role:        string(RoleAdmin),
		JoinedAt:    100,
		ModifiedAt:  100,
	})

	adminSvc := newTestServiceWithRuntime(t, store, 100, admin.Name, ws.Slug)
	description := "team workspace"
	if err := adminSvc.ModifyWorkspace("team", ModifyWorkspaceInput{Description: &description}); err != nil {
		t.Fatalf("ModifyWorkspace() error = %v", err)
	}

	logs, err := adminSvc.ListAudit(AuditListInput{Limit: 10})
	if err != nil {
		t.Fatalf("ListAudit() error = %v", err)
	}
	if len(logs) == 0 || logs[0].Action != "workspace.modify" {
		t.Fatalf("logs = %#v", logs)
	}
}

func TestModifyWorkspaceRejectsEmptyName(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()

	if _, err := svc.AddWorkspace(AddWorkspaceInput{Slug: "team", Name: "Team"}); err != nil {
		t.Fatalf("AddWorkspace() error = %v", err)
	}
	empty := "  "
	if err := svc.ModifyWorkspace("team", ModifyWorkspaceInput{Name: &empty}); err == nil {
		t.Fatal("ModifyWorkspace(empty name) error = nil, want error")
	}
}

func TestWorkspaceAuditUsesTargetWorkspace(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()

	created, err := svc.AddWorkspace(AddWorkspaceInput{Slug: "team", Name: "Team"})
	if err != nil {
		t.Fatalf("AddWorkspace() error = %v", err)
	}
	teamSvc := newTestServiceWithRuntime(t, svc.store, 100, "local", "team")
	logs, err := teamSvc.ListAudit(AuditListInput{Limit: 10})
	if err != nil {
		t.Fatalf("ListAudit(team) error = %v", err)
	}
	if len(logs) == 0 || logs[0].Action != "workspace.add" || logs[0].WorkspaceID == nil || *logs[0].WorkspaceID != created.ID {
		t.Fatalf("team audit logs = %#v", logs)
	}
}

func TestArchiveWorkspaceRejectsWhenAffectedUserHasNoReplacement(t *testing.T) {
	store := newTestStore(t)
	ownerSvc := newTestServiceWithRuntime(t, store, 100, "local", "local")
	ws, err := ownerSvc.AddWorkspace(AddWorkspaceInput{Slug: "solo", Name: "Solo"})
	if err != nil {
		t.Fatalf("AddWorkspace() error = %v", err)
	}

	bob := mustCreateUserRecord(t, store, storage.User{
		ID:                 "user-bob-solo",
		Name:               "bob-solo",
		DefaultWorkspaceID: &ws.ID,
		CreatedAt:          100,
		ModifiedAt:         100,
	})
	mustUpsertMembershipRecord(t, store, storage.Membership{
		UserID:      bob.ID,
		WorkspaceID: ws.ID,
		Role:        string(RoleViewer),
		JoinedAt:    100,
		ModifiedAt:  100,
	})

	if err := ownerSvc.ArchiveWorkspace("solo"); err == nil {
		t.Fatal("ArchiveWorkspace() error = nil, want failure without replacement")
	}
}

func TestArchiveWorkspaceReassignsAffectedUsers(t *testing.T) {
	store := newTestStore(t)
	ownerSvc := newTestServiceWithRuntime(t, store, 100, "local", "local")
	target, err := ownerSvc.AddWorkspace(AddWorkspaceInput{Slug: "old", Name: "Old"})
	if err != nil {
		t.Fatalf("AddWorkspace(old) error = %v", err)
	}
	replacement, err := ownerSvc.AddWorkspace(AddWorkspaceInput{Slug: "new", Name: "New"})
	if err != nil {
		t.Fatalf("AddWorkspace(new) error = %v", err)
	}

	userRepo := storage.NewUserRepository(store.DB())
	memberRepo := storage.NewMemberRepository(store.DB())
	bob := mustCreateUserRecord(t, store, storage.User{
		ID:                 "user-bob-archive",
		Name:               "bob-archive",
		DefaultWorkspaceID: &target.ID,
		CreatedAt:          100,
		ModifiedAt:         100,
	})
	mustUpsertMembershipRecord(t, store, storage.Membership{
		UserID:      bob.ID,
		WorkspaceID: target.ID,
		Role:        string(RoleMember),
		JoinedAt:    100,
		ModifiedAt:  100,
	})
	mustUpsertMembershipRecord(t, store, storage.Membership{
		UserID:      bob.ID,
		WorkspaceID: replacement.ID,
		Role:        string(RoleMember),
		JoinedAt:    100,
		ModifiedAt:  100,
	})
	if err := store.SetMeta(activeWorkspaceMetaKey(bob.ID), target.ID); err != nil {
		t.Fatalf("SetMeta(active workspace) error = %v", err)
	}

	if err := ownerSvc.ArchiveWorkspace("old"); err != nil {
		t.Fatalf("ArchiveWorkspace() error = %v", err)
	}

	reloaded, err := userRepo.GetByID(bob.ID)
	if err != nil {
		t.Fatalf("GetByID(bob) error = %v", err)
	}
	if reloaded.DefaultWorkspaceID == nil || *reloaded.DefaultWorkspaceID != replacement.ID {
		t.Fatalf("bob.DefaultWorkspaceID = %#v, want %q", reloaded.DefaultWorkspaceID, replacement.ID)
	}
	active, ok, err := store.GetMeta(activeWorkspaceMetaKey(bob.ID))
	if err != nil {
		t.Fatalf("GetMeta(active workspace) error = %v", err)
	}
	if !ok || active != replacement.ID {
		t.Fatalf("active workspace = %q, %v; want %q", active, ok, replacement.ID)
	}
	archived, err := storage.NewWorkspaceRepository(store.DB()).GetByID(target.ID)
	if err != nil {
		t.Fatalf("GetByID(old) error = %v", err)
	}
	if archived.ArchivedAt == nil {
		t.Fatal("archived.ArchivedAt = nil, want archived")
	}
	if _, err := memberRepo.Get(bob.ID, replacement.ID); err != nil {
		t.Fatalf("Get(replacement membership) error = %v", err)
	}
}

func TestArchivedWorkspaceRejectsMetadataAndMemberWrites(t *testing.T) {
	store := newTestStore(t)
	ownerSvc := newTestServiceWithRuntime(t, store, 100, "local", "local")
	target, err := ownerSvc.AddWorkspace(AddWorkspaceInput{Slug: "old", Name: "Old"})
	if err != nil {
		t.Fatalf("AddWorkspace(old) error = %v", err)
	}
	if _, err := ownerSvc.AddWorkspace(AddWorkspaceInput{Slug: "new", Name: "New"}); err != nil {
		t.Fatalf("AddWorkspace(new) error = %v", err)
	}
	if err := ownerSvc.ArchiveWorkspace("old"); err != nil {
		t.Fatalf("ArchiveWorkspace() error = %v", err)
	}

	description := "archived"
	if err := ownerSvc.ModifyWorkspace(target.Slug, ModifyWorkspaceInput{Description: &description}); err == nil {
		t.Fatal("ModifyWorkspace(archived) error = nil, want failure")
	}
	alice := mustCreateUserRecord(t, store, storage.User{ID: "user-alice-archived", Name: "alice-archived", CreatedAt: 100, ModifiedAt: 100})
	if err := ownerSvc.AddMember(AddMemberInput{WorkspaceRef: target.Slug, UserRef: alice.Name, Role: RoleViewer}); err == nil {
		t.Fatal("AddMember(archived) error = nil, want failure")
	}
}

func TestAddMemberAndChangeMemberRoleRespectOwnerRules(t *testing.T) {
	store := newTestStore(t)
	ownerSvc := newTestServiceWithRuntime(t, store, 100, "local", "local")
	ws, err := ownerSvc.AddWorkspace(AddWorkspaceInput{Slug: "collab", Name: "Collab"})
	if err != nil {
		t.Fatalf("AddWorkspace() error = %v", err)
	}

	admin := mustCreateUserRecord(t, store, storage.User{ID: "user-admin-collab", Name: "admin-collab", CreatedAt: 100, ModifiedAt: 100})
	alice := mustCreateUserRecord(t, store, storage.User{ID: "user-alice-collab", Name: "alice-collab", CreatedAt: 100, ModifiedAt: 100})
	mustUpsertMembershipRecord(t, store, storage.Membership{
		UserID:      admin.ID,
		WorkspaceID: ws.ID,
		Role:        string(RoleAdmin),
		JoinedAt:    100,
		ModifiedAt:  100,
	})

	adminSvc := newTestServiceWithRuntime(t, store, 100, admin.Name, ws.Slug)
	if err := adminSvc.AddMember(AddMemberInput{WorkspaceRef: ws.Slug, UserRef: alice.Name, Role: RoleViewer}); err != nil {
		t.Fatalf("AddMember() error = %v", err)
	}
	if err := adminSvc.ChangeMemberRole(ChangeMemberRoleInput{WorkspaceRef: ws.Slug, UserRef: alice.Name, Role: RoleOwner}); err == nil {
		t.Fatal("ChangeMemberRole() error = nil, want admin denied for owner promotion")
	}
	if err := ownerSvc.ChangeMemberRole(ChangeMemberRoleInput{WorkspaceRef: ws.Slug, UserRef: alice.Name, Role: RoleOwner}); err != nil {
		t.Fatalf("ChangeMemberRole(owner promote) error = %v", err)
	}
}

func TestChangeMemberRoleProtectsLastOwner(t *testing.T) {
	store := newTestStore(t)
	ownerSvc := newTestServiceWithRuntime(t, store, 100, "local", "local")
	ws, err := ownerSvc.AddWorkspace(AddWorkspaceInput{Slug: "owners", Name: "Owners"})
	if err != nil {
		t.Fatalf("AddWorkspace() error = %v", err)
	}

	if err := ownerSvc.ChangeMemberRole(ChangeMemberRoleInput{WorkspaceRef: ws.Slug, UserRef: "local", Role: RoleAdmin}); err == nil {
		t.Fatal("ChangeMemberRole() error = nil, want last owner protection")
	}
}

func TestUrgencyUsesConfiguredUDACoefficients(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()

	if err := svc.SetConfig("uda.estimate.type", "numeric"); err != nil {
		t.Fatalf("SetConfig(uda type) error = %v", err)
	}
	if err := svc.SetConfig("urgency.uda.estimate.coefficient", "10"); err != nil {
		t.Fatalf("SetConfig(uda coefficient) error = %v", err)
	}
	if err := svc.SetConfig("urgency.uda.estimate.3.coefficient", "7"); err != nil {
		t.Fatalf("SetConfig(uda value coefficient) error = %v", err)
	}
	created, err := svc.Add(AddInput{Description: "estimated", UDAs: map[string]string{"estimate": "3"}})
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	explain, err := svc.ExplainUrgency(created.UUID)
	if err != nil {
		t.Fatalf("ExplainUrgency() error = %v", err)
	}
	if !hasUrgencyItem(explain, "uda.estimate") || !hasUrgencyItem(explain, "uda.estimate.3") {
		t.Fatalf("urgency items = %#v", explain.Items)
	}
	if explain.Total < 17 {
		t.Fatalf("urgency total = %f, want UDA coefficients applied", explain.Total)
	}
}

func TestUrgencyUsesRuntimeUDACoefficients(t *testing.T) {
	store, err := storage.Open(filepath.Join(t.TempDir(), "taskg.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	svc, err := NewService(ServiceOptions{
		Store: store,
		Clock: FixedClock{NowUnix: 100},
		RuntimeConfig: map[string]string{
			"uda.estimate.type":                  "numeric",
			"urgency.uda.estimate.coefficient":   "10",
			"urgency.uda.estimate.3.coefficient": "7",
		},
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	created, err := svc.Add(AddInput{Description: "runtime estimated", UDAs: map[string]string{"estimate": "3"}})
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	explain, err := svc.ExplainUrgency(created.UUID)
	if err != nil {
		t.Fatalf("ExplainUrgency() error = %v", err)
	}
	if !hasUrgencyItem(explain, "uda.estimate") || !hasUrgencyItem(explain, "uda.estimate.3") {
		t.Fatalf("urgency items = %#v", explain.Items)
	}
}

func TestImportPreservesOrphanUDA(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()

	var payload []task.JSONTask
	if err := task.UnmarshalJSONTasks(strings.NewReader(`[{"uuid":"u1","description":"task","status":"pending","entry":"1970-01-01T00:01:40Z","modified":"1970-01-01T00:01:40Z","legacy_field":"old"}]`), &payload); err != nil {
		t.Fatalf("UnmarshalJSONTasks() error = %v", err)
	}
	if _, err := svc.Import(payload); err != nil {
		t.Fatalf("Import() error = %v", err)
	}
	got, err := svc.ResolveTarget("u1")
	if err != nil {
		t.Fatalf("ResolveTarget() error = %v", err)
	}
	legacy := got.UDAs["legacy_field"]
	if legacy.Raw != "old" || !legacy.Orphan {
		t.Fatalf("legacy UDA = %#v", legacy)
	}
	var updated []task.JSONTask
	if err := task.UnmarshalJSONTasks(strings.NewReader(`[{"uuid":"u1","description":"task","status":"pending","entry":"1970-01-01T00:01:40Z","modified":"1970-01-01T00:01:40Z","legacy_field":"new"}]`), &updated); err != nil {
		t.Fatalf("UnmarshalJSONTasks(update) error = %v", err)
	}
	if _, err := svc.Import(updated); err != nil {
		t.Fatalf("Import(update) error = %v", err)
	}
	got, _ = svc.ResolveTarget("u1")
	if got.UDAs["legacy_field"].Raw != "new" {
		t.Fatalf("legacy UDA after update = %#v", got.UDAs["legacy_field"])
	}
}

func TestModifyRejectsOrphanUDA(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()
	var payload []task.JSONTask
	if err := task.UnmarshalJSONTasks(strings.NewReader(`[{"uuid":"u1","description":"task","status":"pending","entry":"1970-01-01T00:01:40Z","modified":"1970-01-01T00:01:40Z","legacy_field":"old"}]`), &payload); err != nil {
		t.Fatalf("UnmarshalJSONTasks() error = %v", err)
	}
	if _, err := svc.Import(payload); err != nil {
		t.Fatalf("Import() error = %v", err)
	}
	if err := svc.Modify("u1", ModifyInput{UDAs: map[string]string{"legacy_field": "new"}}); err == nil {
		t.Fatal("Modify(orphan set) error = nil, want error")
	}
	if err := svc.Modify("u1", ModifyInput{ClearUDAs: []string{"legacy_field"}}); err == nil {
		t.Fatal("Modify(orphan clear) error = nil, want error")
	}
}

func TestUniqueHelperSupportsUDA(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()
	if err := svc.DefineUDA("estimate", "numeric", "Estimate", nil, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Add(AddInput{Description: "one", UDAs: map[string]string{"estimate": "3"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Add(AddInput{Description: "two", UDAs: map[string]string{"estimate": "5"}}); err != nil {
		t.Fatal(err)
	}
	values, err := svc.UniqueValues("estimate", ListInput{})
	if err != nil {
		t.Fatalf("UniqueValues() error = %v", err)
	}
	if strings.Join(values, ",") != "3,5" {
		t.Fatalf("UniqueValues() = %#v", values)
	}
}

func TestProjectsUsesProjectTableNotTaskAggregation(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()

	if _, err := svc.AddProject(AddProjectInput{Slug: "api", Name: "API"}); err != nil {
		t.Fatalf("AddProject(api) error = %v", err)
	}
	project, err := svc.AddProject(AddProjectInput{Slug: "legacy", Name: "Legacy"})
	if err != nil {
		t.Fatalf("AddProject(legacy) error = %v", err)
	}
	if _, err := svc.ArchiveProject(project.ID); err != nil {
		t.Fatalf("ArchiveProject(legacy) error = %v", err)
	}

	active, err := svc.Projects(false)
	if err != nil {
		t.Fatalf("Projects(false) error = %v", err)
	}
	if strings.Join(active, ",") != "api" {
		t.Fatalf("Projects(false) = %#v, want only active project without tasks", active)
	}

	all, err := svc.Projects(true)
	if err != nil {
		t.Fatalf("Projects(true) error = %v", err)
	}
	if strings.Join(all, ",") != "api,legacy" {
		t.Fatalf("Projects(true) = %#v, want active+archived from project table", all)
	}
}

func TestUniqueValuesProjectUsesValidatedBindingsOnly(t *testing.T) {
	store := newTestStore(t)
	svc := newTestServiceWithRuntime(t, store, 100, "local", "local")

	project, err := svc.AddProject(AddProjectInput{Slug: "api", Name: "API"})
	if err != nil {
		t.Fatalf("AddProject(api) error = %v", err)
	}
	created, err := svc.Add(AddInput{Description: "project task", Project: &project.Slug})
	if err != nil {
		t.Fatalf("Add(task) error = %v", err)
	}
	values, err := svc.UniqueValues("project", ListInput{})
	if err != nil {
		t.Fatalf("UniqueValues(project) error = %v", err)
	}
	if strings.Join(values, ",") != "api" {
		t.Fatalf("UniqueValues(project) = %#v, want api", values)
	}

	if err := store.DB().Model(&storage.Task{}).
		Where("uuid = ? AND workspace_id = ?", created.UUID, svc.Runtime().WorkspaceID).
		Update("project", "broken").Error; err != nil {
		t.Fatalf("corrupt task project slug error = %v", err)
	}
	if _, err := svc.UniqueValues("project", ListInput{}); err == nil {
		t.Fatal("UniqueValues(project) error = nil, want project_invariant_violation")
	} else if runtimeErr, ok := err.(RuntimeError); !ok || runtimeErr.Code != "project_invariant_violation" {
		t.Fatalf("UniqueValues(project) err = %#v, want RuntimeError(project_invariant_violation)", err)
	}
}

func TestTaskRCDryRunDoesNotWriteState(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()
	path := filepath.Join(t.TempDir(), ".taskrc")
	if err := os.WriteFile(path, []byte("context.work=project:work\nuda.estimate.type=numeric\nurgency.uda.estimate.coefficient=2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	report, err := svc.ImportTaskRC(path, true)
	if err != nil {
		t.Fatalf("ImportTaskRC(dry-run) error = %v", err)
	}
	if !report.DryRun || len(report.Imported) != 3 {
		t.Fatalf("dry-run report = %#v", report)
	}
	if contexts, err := svc.ContextList(); err != nil || len(contexts) != 0 {
		t.Fatalf("contexts after dry-run = %#v, %v", contexts, err)
	}
	if defs, err := svc.ListUDAs(); err != nil || len(defs) != 0 {
		t.Fatalf("UDAs after dry-run = %#v, %v", defs, err)
	}
	report, err = svc.ImportTaskRC(path, false)
	if err != nil {
		t.Fatalf("ImportTaskRC() error = %v", err)
	}
	if report.DryRun {
		t.Fatalf("report dry_run = true")
	}
	if contexts, _ := svc.ContextList(); len(contexts) != 1 || contexts[0].Name != "work" {
		t.Fatalf("contexts after import = %#v", contexts)
	}
	if got, ok, err := svc.GetConfig("urgency.uda.estimate.coefficient"); err != nil || !ok || got != "2" {
		t.Fatalf("urgency config = %q, %v, %v", got, ok, err)
	}
	if got, ok, err := svc.GetConfig("uda.estimate.type"); err != nil || !ok || got != "numeric" {
		t.Fatalf("uda type = %q, %v, %v", got, ok, err)
	}
}

func TestImportTaskRCAppliesActiveContextSelection(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()
	path := filepath.Join(t.TempDir(), ".taskrc")
	if err := os.WriteFile(path, []byte(strings.Join([]string{
		"context.work=project:work",
		"context.active=work",
		"",
	}, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}

	report, err := svc.ImportTaskRC(path, false)
	if err != nil {
		t.Fatalf("ImportTaskRC() error = %v", err)
	}
	if !hasTaskRCImportedTarget(report.Imported, "context.active") {
		t.Fatalf("report imported missing context.active: %#v", report.Imported)
	}
	show, err := svc.ContextShow()
	if err != nil {
		t.Fatalf("ContextShow() error = %v", err)
	}
	if !strings.Contains(show, "work") || !strings.Contains(show, "project:work") {
		t.Fatalf("ContextShow() = %q, want active imported context", show)
	}
}

func TestContextDefineUseShowNoneDelete(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()

	if show, err := svc.ContextShow(); err != nil || show != "" {
		t.Fatalf("empty ContextShow() = %q, %v", show, err)
	}
	if err := svc.DefineContext("work", "project:work"); err != nil {
		t.Fatalf("DefineContext() error = %v", err)
	}
	if err := svc.UseContext("work"); err != nil {
		t.Fatalf("UseContext() error = %v", err)
	}
	show, err := svc.ContextShow()
	if err != nil {
		t.Fatalf("ContextShow() error = %v", err)
	}
	if !strings.Contains(show, "work") || !strings.Contains(show, "project:work") {
		t.Fatalf("ContextShow() = %q", show)
	}
	if err := svc.ContextNone(); err != nil {
		t.Fatalf("ContextNone() error = %v", err)
	}
	if show, err := svc.ContextShow(); err != nil || show != "" {
		t.Fatalf("ContextShow() after none = %q, %v", show, err)
	}
	if err := svc.UseContext("work"); err != nil {
		t.Fatalf("UseContext(work) error = %v", err)
	}
	if err := svc.ContextDelete("work"); err != nil {
		t.Fatalf("ContextDelete() error = %v", err)
	}
	if show, err := svc.ContextShow(); err != nil || show != "" {
		t.Fatalf("ContextShow() after delete active = %q, %v", show, err)
	}
}

func TestContextNonePersistsEmptyOverrideOverRuntimeConfig(t *testing.T) {
	store, err := storage.Open(filepath.Join(t.TempDir(), "taskg.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	svc, err := NewService(ServiceOptions{
		Store:         store,
		Clock:         FixedClock{NowUnix: 100},
		RuntimeConfig: map[string]string{"context.active": "work"},
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	if err := svc.DefineContext("work", "project:work"); err != nil {
		t.Fatalf("DefineContext() error = %v", err)
	}
	if show, err := svc.ContextShow(); err != nil || show != "" {
		t.Fatalf("ContextShow() with runtime config = %q, %v; want no TOML/runtime fallback", show, err)
	}
	if err := svc.ContextNone(); err != nil {
		t.Fatalf("ContextNone() error = %v", err)
	}
	if show, err := svc.ContextShow(); err != nil || show != "" {
		t.Fatalf("ContextShow() after none = %q, %v", show, err)
	}
}

func TestContextFilterAppliesToListAndReports(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()
	if _, err := svc.AddProject(AddProjectInput{Slug: "work", Name: "Work"}); err != nil {
		t.Fatalf("AddProject(work) error = %v", err)
	}
	if _, err := svc.AddProject(AddProjectInput{Slug: "home", Name: "Home"}); err != nil {
		t.Fatalf("AddProject(home) error = %v", err)
	}
	if _, err := svc.Add(AddInput{Description: "work task", Project: strptr("work")}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Add(AddInput{Description: "home task", Project: strptr("home")}); err != nil {
		t.Fatal(err)
	}
	if err := svc.DefineContext("work", "project:work"); err != nil {
		t.Fatal(err)
	}
	if err := svc.UseContext("work"); err != nil {
		t.Fatal(err)
	}
	tasks, err := svc.List(ListInput{})
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 || tasks[0].Description != "work task" {
		t.Fatalf("List with context = %#v", tasks)
	}
	all, err := svc.RunReport(ReportInput{Name: "all"})
	if err != nil {
		t.Fatal(err)
	}
	if len(all.Tasks) != 1 || all.Tasks[0].Description != "work task" {
		t.Fatalf("all report with context = %#v", all.Tasks)
	}
}

func TestProjectQueryResolvesWithinCurrentWorkspace(t *testing.T) {
	store := newTestStore(t)
	ownerLocal := newTestServiceWithRuntime(t, store, 100, "local", "local")
	localProject, err := ownerLocal.AddProject(AddProjectInput{Slug: "same", Name: "Local Same"})
	if err != nil {
		t.Fatalf("local AddProject(same) error = %v", err)
	}
	if _, err := ownerLocal.Add(AddInput{Description: "local task", Project: &localProject.Slug}); err != nil {
		t.Fatalf("local Add(task) error = %v", err)
	}

	work, err := ownerLocal.AddWorkspace(AddWorkspaceInput{Slug: "work", Name: "Work"})
	if err != nil {
		t.Fatalf("AddWorkspace(work) error = %v", err)
	}
	ownerWork := newTestServiceWithRuntime(t, store, 100, "local", work.Slug)
	workProject, err := ownerWork.AddProject(AddProjectInput{Slug: "same", Name: "Work Same"})
	if err != nil {
		t.Fatalf("work AddProject(same) error = %v", err)
	}
	workTask, err := ownerWork.Add(AddInput{Description: "work task", Project: &workProject.Slug})
	if err != nil {
		t.Fatalf("work Add(task) error = %v", err)
	}

	expr, err := query.ParseQuery(`project:same`)
	if err != nil {
		t.Fatalf("ParseQuery(project:same) error = %v", err)
	}
	tasks, err := ownerWork.List(ListInput{Query: expr})
	if err != nil {
		t.Fatalf("List(project:same) error = %v", err)
	}
	if len(tasks) != 1 || tasks[0].UUID != workTask.UUID {
		t.Fatalf("List(project:same) tasks = %#v, want only work task", tasks)
	}
}

func TestProjectQueryMissingReturnsStableError(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()

	expr, err := query.ParseQuery(`project:missing`)
	if err != nil {
		t.Fatalf("ParseQuery(project:missing) error = %v", err)
	}
	if _, err := svc.List(ListInput{Query: expr}); err == nil {
		t.Fatal("List(project:missing) error = nil, want project_not_found")
	} else if runtimeErr, ok := err.(RuntimeError); !ok || runtimeErr.Code != "project_not_found" {
		t.Fatalf("List(project:missing) err = %#v, want RuntimeError(project_not_found)", err)
	}
}

func TestContextFilterResolvesProjectWithinWorkspace(t *testing.T) {
	store := newTestStore(t)
	ownerLocal := newTestServiceWithRuntime(t, store, 100, "local", "local")
	if _, err := ownerLocal.AddProject(AddProjectInput{Slug: "same", Name: "Local Same"}); err != nil {
		t.Fatalf("local AddProject(same) error = %v", err)
	}
	if _, err := ownerLocal.Add(AddInput{Description: "local task", Project: strptr("same")}); err != nil {
		t.Fatalf("local Add(task) error = %v", err)
	}

	work, err := ownerLocal.AddWorkspace(AddWorkspaceInput{Slug: "work", Name: "Work"})
	if err != nil {
		t.Fatalf("AddWorkspace(work) error = %v", err)
	}
	ownerWork := newTestServiceWithRuntime(t, store, 100, "local", work.Slug)
	if _, err := ownerWork.AddProject(AddProjectInput{Slug: "same", Name: "Work Same"}); err != nil {
		t.Fatalf("work AddProject(same) error = %v", err)
	}
	workTask, err := ownerWork.Add(AddInput{Description: "work task", Project: strptr("same")})
	if err != nil {
		t.Fatalf("work Add(task) error = %v", err)
	}
	if err := ownerWork.DefineContext("same", "project:same"); err != nil {
		t.Fatalf("DefineContext(project:same) error = %v", err)
	}
	if err := ownerWork.UseContext("same"); err != nil {
		t.Fatalf("UseContext(same) error = %v", err)
	}
	tasks, err := ownerWork.List(ListInput{})
	if err != nil {
		t.Fatalf("List() with project context error = %v", err)
	}
	if len(tasks) != 1 || tasks[0].UUID != workTask.UUID {
		t.Fatalf("List() with project context tasks = %#v, want only work task", tasks)
	}
}

func TestNoContextBypassesActiveContext(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()
	if _, err := svc.AddProject(AddProjectInput{Slug: "work", Name: "Work"}); err != nil {
		t.Fatalf("AddProject(work) error = %v", err)
	}
	if _, err := svc.AddProject(AddProjectInput{Slug: "home", Name: "Home"}); err != nil {
		t.Fatalf("AddProject(home) error = %v", err)
	}
	if _, err := svc.Add(AddInput{Description: "work task", Project: strptr("work")}); err != nil {
		t.Fatalf("Add(work task) error = %v", err)
	}
	if _, err := svc.Add(AddInput{Description: "home task", Project: strptr("home")}); err != nil {
		t.Fatalf("Add(home task) error = %v", err)
	}
	if err := svc.DefineContext("work", "project:work"); err != nil {
		t.Fatal(err)
	}
	if err := svc.UseContext("work"); err != nil {
		t.Fatal(err)
	}
	tasks, err := svc.List(ListInput{NoContext: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 2 {
		t.Fatalf("List(NoContext) len = %d, want 2: %#v", len(tasks), tasks)
	}
}

func TestSetConfigRejectsUnsupportedBusinessKey(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()

	err := svc.SetConfig("arbitrary.thing", "1")
	if err == nil {
		t.Fatal("SetConfig(arbitrary.thing) succeeded unexpectedly")
	}
	assertRuntimeCode(t, err, "config_key_unsupported")
}

func TestSetConfigPersonalKeysSkipAuditAndRole(t *testing.T) {
	store := newTestStore(t)
	ownerSvc := newTestServiceWithRuntime(t, store, 100, "local", "local")
	viewer, err := ownerSvc.AddUser(AddUserInput{Name: "viewer"})
	if err != nil {
		t.Fatal(err)
	}
	if err := ownerSvc.AddMember(AddMemberInput{WorkspaceRef: "local", UserRef: viewer.ID, Role: RoleViewer}); err != nil {
		t.Fatal(err)
	}
	before, err := ownerSvc.ListAudit(AuditListInput{Limit: 50})
	if err != nil {
		t.Fatal(err)
	}

	viewerSvc := newTestServiceWithRuntime(t, store, 100, "viewer", "local")
	if err := viewerSvc.SetConfig("color", "false"); err != nil {
		t.Fatalf("viewer SetConfig(color) error = %v", err)
	}
	if err := viewerSvc.SetConfig("json", "true"); err != nil {
		t.Fatalf("viewer SetConfig(json) error = %v", err)
	}
	if value, ok, err := viewerSvc.GetConfig("color"); err != nil || !ok || value != "false" {
		t.Fatalf("GetConfig(color) = %q, %v, %v; want false, true, nil", value, ok, err)
	}
	if err := viewerSvc.UnsetConfig("color"); err != nil {
		t.Fatalf("viewer UnsetConfig(color) error = %v", err)
	}
	if value, ok, err := viewerSvc.GetConfig("color"); err != nil || ok || value != "" {
		t.Fatalf("GetConfig(color after unset) = %q, %v, %v; want empty, false, nil", value, ok, err)
	}
	if err := viewerSvc.SetConfig("date.format", "epoch"); err == nil {
		t.Fatal("viewer SetConfig(date.format) succeeded unexpectedly")
	}

	after, err := ownerSvc.ListAudit(AuditListInput{Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(before) {
		t.Fatalf("personal render config wrote audit rows: before=%d after=%d rows=%#v", len(before), len(after), after)
	}
}

func TestExportWithInputFiltersByProjectID(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()

	alpha, err := svc.AddProject(AddProjectInput{Slug: "alpha", Name: "Alpha"})
	if err != nil {
		t.Fatal(err)
	}
	beta, err := svc.AddProject(AddProjectInput{Slug: "beta", Name: "Beta"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Add(AddInput{Description: "alpha task", Project: strptr("alpha")}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Add(AddInput{Description: "beta task", Project: strptr("beta")}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Add(AddInput{Description: "no project"}); err != nil {
		t.Fatal(err)
	}

	missingProjectID := "00000000-0000-0000-0000-000000000000"
	cases := []struct {
		name      string
		projectID *string
		want      []string
	}{
		{name: "no filter", want: []string{"alpha task", "beta task", "no project"}},
		{name: "alpha only", projectID: &alpha.ID, want: []string{"alpha task"}},
		{name: "missing project", projectID: &missingProjectID},
	}
	_ = beta
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tasks, err := svc.ExportWithInput(ExportInput{ProjectID: tc.projectID})
			if err != nil {
				t.Fatal(err)
			}
			got := make([]string, 0, len(tasks))
			for _, tsk := range tasks {
				got = append(got, tsk.Description)
			}
			sort.Strings(got)
			want := append([]string(nil), tc.want...)
			sort.Strings(want)
			if !slices.Equal(got, want) {
				t.Fatalf("ExportWithInput descriptions = %v, want %v", got, want)
			}
		})
	}
}

func TestServiceModifyDoneDeleteByNumber(t *testing.T) {
	store, err := storage.Open(filepath.Join(t.TempDir(), "taskg.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	svc, _ := NewService(ServiceOptions{Store: store, Clock: FixedClock{NowUnix: 100}})

	_, err = svc.Add(AddInput{Description: "write spec"})
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	priority := "H"
	if err := svc.Modify("1", ModifyInput{Priority: &priority}); err != nil {
		t.Fatalf("Modify() error = %v", err)
	}
	got, err := svc.ResolveTarget("1")
	if err != nil {
		t.Fatalf("ResolveTarget() error = %v", err)
	}
	if got.Priority == nil || *got.Priority != "H" {
		t.Fatalf("Priority = %#v", got.Priority)
	}
	if err := svc.Done("1"); err != nil {
		t.Fatalf("Done() error = %v", err)
	}
	tasks, _ := svc.List(ListInput{})
	if len(tasks) != 0 {
		t.Fatalf("pending tasks = %#v, want empty", tasks)
	}
}

func TestServiceM2Mutations(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()
	dep, _ := svc.Add(AddInput{Description: "dep"})
	tsk, _ := svc.Add(AddInput{Description: "task"})

	if err := svc.Modify(tsk.UUID, ModifyInput{AddDepends: []string{dep.UUID}}); err != nil {
		t.Fatalf("Modify(depends) error = %v", err)
	}
	if err := svc.Start(tsk.UUID); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if err := svc.Annotate(tsk.UUID, "note"); err != nil {
		t.Fatalf("Annotate() error = %v", err)
	}
	if err := svc.AppendDescription(tsk.UUID, "suffix"); err != nil {
		t.Fatalf("AppendDescription() error = %v", err)
	}
	if err := svc.PrependDescription(tsk.UUID, "prefix"); err != nil {
		t.Fatalf("PrependDescription() error = %v", err)
	}
	got, err := svc.ResolveTarget(tsk.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Start == nil || len(got.Depends) != 1 || len(got.Annotations) != 1 || got.Description != "prefix task suffix" {
		t.Fatalf("M2 fields not updated: %#v", got)
	}
	if err := svc.Stop(tsk.UUID); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	got, _ = svc.ResolveTarget(tsk.UUID)
	if got.Start != nil {
		t.Fatalf("Start after Stop = %#v", got.Start)
	}
}

func TestServiceRejectsDependencyCycle(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()
	a, _ := svc.Add(AddInput{Description: "a"})
	b, _ := svc.Add(AddInput{Description: "b"})
	if err := svc.Modify(a.UUID, ModifyInput{AddDepends: []string{b.UUID}}); err != nil {
		t.Fatalf("Modify(a depends b) error = %v", err)
	}
	if err := svc.Modify(b.UUID, ModifyInput{AddDepends: []string{a.UUID}}); err == nil {
		t.Fatal("expected dependency cycle error")
	}
}

func TestServiceAddResolvesDependencyTargets(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()
	dep, _ := svc.Add(AddInput{Description: "dep"})
	tsk, err := svc.Add(AddInput{Description: "task", Depends: []string{"1"}})
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	got, err := svc.ResolveTarget(tsk.UUID)
	if err != nil {
		t.Fatalf("ResolveTarget() error = %v", err)
	}
	if len(got.Depends) != 1 || got.Depends[0] != dep.UUID {
		t.Fatalf("Depends = %#v, want %q", got.Depends, dep.UUID)
	}
}

func TestServiceRejectsRecurringUnsupportedFields(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()
	dep, _ := svc.Add(AddInput{Description: "dep"})
	due := int64(200)
	wait := int64(150)
	scheduled := int64(160)
	recur := "daily"
	for _, tc := range []struct {
		name  string
		input AddInput
	}{
		{name: "wait", input: AddInput{Description: "task", Due: &due, Recur: &recur, Wait: &wait}},
		{name: "scheduled", input: AddInput{Description: "task", Due: &due, Recur: &recur, Scheduled: &scheduled}},
		{name: "depends", input: AddInput{Description: "task", Due: &due, Recur: &recur, Depends: []string{dep.UUID}}},
	} {
		if _, err := svc.Add(tc.input); err == nil {
			t.Fatalf("Add(%s) error = nil, want error", tc.name)
		}
	}
}

func TestServiceRejectsDeepDependencyCycle(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()
	a, _ := svc.Add(AddInput{Description: "a"})
	b, _ := svc.Add(AddInput{Description: "b"})
	c, _ := svc.Add(AddInput{Description: "c"})
	d, _ := svc.Add(AddInput{Description: "d"})
	if err := svc.Modify(a.UUID, ModifyInput{AddDepends: []string{b.UUID}}); err != nil {
		t.Fatalf("Modify(a depends b) error = %v", err)
	}
	if err := svc.Modify(b.UUID, ModifyInput{AddDepends: []string{c.UUID}}); err != nil {
		t.Fatalf("Modify(b depends c) error = %v", err)
	}
	if err := svc.Modify(c.UUID, ModifyInput{AddDepends: []string{d.UUID}}); err != nil {
		t.Fatalf("Modify(c depends d) error = %v", err)
	}
	if err := svc.Modify(d.UUID, ModifyInput{AddDepends: []string{a.UUID}}); err == nil {
		t.Fatal("expected deep dependency cycle error")
	}
}

func TestServiceModifyClearDependsBeforeAdding(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()
	dep1, _ := svc.Add(AddInput{Description: "dep1"})
	dep2, _ := svc.Add(AddInput{Description: "dep2"})
	tsk, _ := svc.Add(AddInput{Description: "task"})
	if err := svc.Modify(tsk.UUID, ModifyInput{AddDepends: []string{dep1.UUID}}); err != nil {
		t.Fatalf("Modify(initial depends) error = %v", err)
	}
	if err := svc.Modify(tsk.UUID, ModifyInput{ClearDepends: true, AddDepends: []string{dep2.UUID}}); err != nil {
		t.Fatalf("Modify(clear then add depends) error = %v", err)
	}
	got, err := svc.ResolveTarget(tsk.UUID)
	if err != nil {
		t.Fatalf("ResolveTarget() error = %v", err)
	}
	if len(got.Depends) != 1 || got.Depends[0] != dep2.UUID {
		t.Fatalf("Depends = %#v, want only %q", got.Depends, dep2.UUID)
	}
}

func TestServiceRejectsBlankAnnotateAppendAndPrepend(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()
	tsk, _ := svc.Add(AddInput{Description: "task"})
	for _, tc := range []struct {
		name string
		run  func() error
	}{
		{name: "annotate", run: func() error { return svc.Annotate(tsk.UUID, " \t ") }},
		{name: "append", run: func() error { return svc.AppendDescription(tsk.UUID, "   ") }},
		{name: "prepend", run: func() error { return svc.PrependDescription(tsk.UUID, "\n\t") }},
	} {
		if err := tc.run(); err == nil {
			t.Fatalf("%s() error = nil, want error", tc.name)
		}
	}
	got, err := svc.ResolveTarget(tsk.UUID)
	if err != nil {
		t.Fatalf("ResolveTarget() error = %v", err)
	}
	if got.Description != "task" {
		t.Fatalf("Description = %q, want unchanged", got.Description)
	}
	if len(got.Annotations) != 0 {
		t.Fatalf("Annotations = %#v, want empty", got.Annotations)
	}
}

func TestServiceRejectsAnnotationNewline(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()
	tsk, _ := svc.Add(AddInput{Description: "task"})
	if err := svc.Annotate(tsk.UUID, "line1\nline2"); err == nil {
		t.Fatal("Annotate() error = nil, want newline validation error")
	}
	got, err := svc.ResolveTarget(tsk.UUID)
	if err != nil {
		t.Fatalf("ResolveTarget() error = %v", err)
	}
	if len(got.Annotations) != 0 {
		t.Fatalf("Annotations = %#v, want empty", got.Annotations)
	}
}

func TestServiceAllowsDuplicateAnnotationsInSameSecond(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()
	tsk, _ := svc.Add(AddInput{Description: "task"})
	if err := svc.Annotate(tsk.UUID, "note"); err != nil {
		t.Fatalf("Annotate(first) error = %v", err)
	}
	if err := svc.Annotate(tsk.UUID, "note"); err != nil {
		t.Fatalf("Annotate(second) error = %v", err)
	}
	got, err := svc.ResolveTarget(tsk.UUID)
	if err != nil {
		t.Fatalf("ResolveTarget() error = %v", err)
	}
	if len(got.Annotations) != 2 {
		t.Fatalf("Annotations = %#v, want two duplicate notes", got.Annotations)
	}
	if got.Annotations[0].Entry == got.Annotations[1].Entry {
		t.Fatalf("duplicate annotations kept same entry: %#v", got.Annotations)
	}
}

func TestServiceDeleteAndStopRejectTerminalStates(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()
	completed, err := svc.Add(AddInput{Description: "done task"})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Done(completed.UUID); err != nil {
		t.Fatalf("Done() error = %v", err)
	}
	if err := svc.Delete(completed.UUID); err == nil {
		t.Fatal("Delete(completed) error = nil, want terminal-state guard")
	}
	if err := svc.Stop(completed.UUID); err == nil {
		t.Fatal("Stop(completed) error = nil, want terminal-state guard")
	}

	deleted, err := svc.Add(AddInput{Description: "deleted task"})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(deleted.UUID); err != nil {
		t.Fatalf("Delete(first) error = %v", err)
	}
	if err := svc.Delete(deleted.UUID); err == nil {
		t.Fatal("Delete(deleted) error = nil, want terminal-state guard")
	}
	if err := svc.Stop(deleted.UUID); err == nil {
		t.Fatal("Stop(deleted) error = nil, want terminal-state guard")
	}
}

func TestServiceImportClearsTagsWithExplicitEmptyArray(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()
	created, err := svc.Add(AddInput{Description: "task", Tags: []string{"one", "two"}})
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	if _, err := svc.Import([]task.JSONTask{{
		UUID:        created.UUID,
		Description: created.Description,
		Status:      task.StatusPending,
		Entry:       "1970-01-01T00:01:40Z",
		Modified:    "1970-01-01T00:01:40Z",
		Tags:        []string{},
	}}); err != nil {
		t.Fatalf("Import() error = %v", err)
	}
	got, err := svc.ResolveTarget(created.UUID)
	if err != nil {
		t.Fatalf("ResolveTarget() error = %v", err)
	}
	if got.Tags == nil {
		t.Fatal("Tags = nil, want explicit empty slice after clearing")
	}
	if len(got.Tags) != 0 {
		t.Fatalf("Tags = %#v, want empty", got.Tags)
	}
}

func TestServiceImportDoesNotClearTagsWhenFieldMissing(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()
	created, err := svc.Add(AddInput{Description: "task", Tags: []string{"one", "two"}})
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	raw := `[
		{
			"uuid": "` + created.UUID + `",
			"description": "task",
			"status": "pending",
			"entry": "1970-01-01T00:01:40Z",
			"modified": "1970-01-01T00:01:40Z"
		}
	]`
	var payload []task.JSONTask
	if err := task.UnmarshalJSONTasks(strings.NewReader(raw), &payload); err != nil {
		t.Fatalf("UnmarshalJSONTasks() error = %v", err)
	}
	if _, err := svc.Import(payload); err != nil {
		t.Fatalf("Import() error = %v", err)
	}
	got, err := svc.ResolveTarget(created.UUID)
	if err != nil {
		t.Fatalf("ResolveTarget() error = %v", err)
	}
	if len(got.Tags) != 2 {
		t.Fatalf("Tags = %#v, want preserved tags", got.Tags)
	}
}

func TestServiceImportIsAtomicOnFailure(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()

	payload := []task.JSONTask{
		{
			UUID:        "ok-1",
			Description: "first",
			Status:      task.StatusPending,
			Entry:       "1970-01-01T00:01:40Z",
			Modified:    "1970-01-01T00:01:40Z",
		},
		{
			UUID:        "bad-2",
			Description: "second",
			Status:      task.StatusPending,
			Entry:       "not-a-date",
			Modified:    "1970-01-01T00:01:40Z",
		},
	}

	count, err := svc.Import(payload)
	if err == nil {
		t.Fatal("Import() error = nil, want rollback on invalid batch")
	}
	if count != 0 {
		t.Fatalf("Import() count = %d, want 0 on atomic rollback", count)
	}
	tasks, listErr := svc.Export()
	if listErr != nil {
		t.Fatalf("Export() error = %v", listErr)
	}
	if len(tasks) != 0 {
		t.Fatalf("tasks persisted after failed import: %#v", tasks)
	}
}

func hasTaskRCImportedTarget(entries []taskrcparser.Entry, target string) bool {
	for _, entry := range entries {
		if entry.Target == target || entry.Key == target {
			return true
		}
	}
	return false
}

func TestDefaultWorkingSetKeepsWaitingTasksAddressable(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()
	wait := int64(200)
	if _, err := svc.Add(AddInput{Description: "hidden wait", Wait: &wait}); err != nil {
		t.Fatalf("Add(waiting) error = %v", err)
	}
	visible, err := svc.Add(AddInput{Description: "visible"})
	if err != nil {
		t.Fatalf("Add(visible) error = %v", err)
	}
	got, err := svc.ResolveTarget("1")
	if err != nil {
		t.Fatalf("ResolveTarget(1) error = %v", err)
	}
	if got.UUID == visible.UUID {
		t.Fatalf("ResolveTarget(1) should keep waiting task addressable before visible %q", visible.Description)
	}
}

func TestServiceM2Reports(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()
	waitUntil := int64(200)
	expiredUntil := int64(90)
	active, _ := svc.Add(AddInput{Description: "active"})
	waiting, _ := svc.Add(AddInput{Description: "waiting", Wait: &waitUntil})
	expired, _ := svc.Add(AddInput{Description: "expired", Until: &expiredUntil})
	dep, _ := svc.Add(AddInput{Description: "dep"})
	blocked, _ := svc.Add(AddInput{Description: "blocked"})
	if err := svc.Start(active.UUID); err != nil {
		t.Fatalf("Start(active) error = %v", err)
	}
	if err := svc.Modify(blocked.UUID, ModifyInput{AddDepends: []string{dep.UUID}}); err != nil {
		t.Fatalf("Modify(blocked depends) error = %v", err)
	}

	cases := map[string]string{
		"active":   active.UUID,
		"waiting":  waiting.UUID,
		"blocked":  blocked.UUID,
		"blocking": dep.UUID,
	}
	for reportName, wantUUID := range cases {
		got, err := svc.ListReport(reportName, ListInput{})
		if err != nil {
			t.Fatalf("ListReport(%s) error = %v", reportName, err)
		}
		if !containsTask(got, wantUUID) {
			t.Fatalf("ListReport(%s) = %#v, missing %s", reportName, got, wantUUID)
		}
		if containsTask(got, expired.UUID) {
			t.Fatalf("ListReport(%s) includes expired until task: %#v", reportName, got)
		}
	}
	all, err := svc.ListReport("all", ListInput{})
	if err != nil {
		t.Fatalf("ListReport(all) error = %v", err)
	}
	if !containsTask(all, expired.UUID) {
		t.Fatalf("all should include until-expired task: %#v", all)
	}
}

func TestUntilExpiredDependencyDoesNotBlockLiveTask(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()
	expiredUntil := int64(90)
	expired, _ := svc.Add(AddInput{Description: "expired", Until: &expiredUntil})
	live, _ := svc.Add(AddInput{Description: "live"})
	if err := svc.Modify(live.UUID, ModifyInput{AddDepends: []string{expired.UUID}}); err != nil {
		t.Fatalf("Modify(live depends expired) error = %v", err)
	}
	blocked, err := svc.ListReport("blocked", ListInput{})
	if err != nil {
		t.Fatalf("ListReport(blocked) error = %v", err)
	}
	if containsTask(blocked, live.UUID) {
		t.Fatalf("blocked includes live task with expired dependency: %#v", blocked)
	}
	ready, err := svc.ListReport("ready", ListInput{})
	if err != nil {
		t.Fatalf("ListReport(ready) error = %v", err)
	}
	if !containsTask(ready, live.UUID) {
		t.Fatalf("ready missing live task with expired dependency: %#v", ready)
	}
}

func TestUrgencyUsesDependencyState(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()
	blocker, _ := svc.Add(AddInput{Description: "blocker"})
	blocked, _ := svc.Add(AddInput{Description: "blocked"})
	plain, _ := svc.Add(AddInput{Description: "plain"})
	if err := svc.Modify(blocked.UUID, ModifyInput{AddDepends: []string{blocker.UUID}}); err != nil {
		t.Fatal(err)
	}
	blockedU, err := svc.ExplainUrgency(blocked.UUID)
	if err != nil {
		t.Fatal(err)
	}
	plainU, err := svc.ExplainUrgency(plain.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if blockedU.Total >= plainU.Total {
		t.Fatalf("blocked urgency = %.3f, plain = %.3f; blocked should be lower", blockedU.Total, plainU.Total)
	}
}

func TestDoneChildDoesNotRecurWhenParentDeleted(t *testing.T) {
	svc, closeFn := newTestService(t, mustUnix(t, "2030-01-01T10:00:00Z"))
	defer closeFn()
	due := mustUnix(t, "2030-01-01T23:59:59Z")
	until := mustUnix(t, "2030-02-01T23:59:59Z")
	recur := "daily"
	parent, err := svc.Add(AddInput{Description: "daily task", Due: &due, Until: &until, Recur: &recur})
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	tasks, err := svc.List(ListInput{})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("children = %#v", tasks)
	}
	child := tasks[0]
	if err := svc.Delete(parent.UUID); err != nil {
		t.Fatalf("Delete(parent) error = %v", err)
	}
	if err := svc.Done(child.UUID); err != nil {
		t.Fatalf("Done(child) error = %v", err)
	}
	tasks, err = svc.List(ListInput{})
	if err != nil {
		t.Fatalf("List() after Done(child) error = %v", err)
	}
	if len(tasks) != 0 {
		t.Fatalf("child generated after deleted parent: %#v", tasks)
	}
}

func TestServiceRecurringAddAndDoneCreatesNextChild(t *testing.T) {
	svc, closeFn := newTestService(t, mustUnix(t, "2030-01-01T10:00:00Z"))
	defer closeFn()
	due := mustUnix(t, "2030-01-01T23:59:59Z")
	until := mustUnix(t, "2030-02-01T23:59:59Z")
	recur := "daily"
	parent, err := svc.Add(AddInput{Description: "daily task", Due: &due, Until: &until, Recur: &recur})
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	if parent.Status != task.StatusRecurring {
		t.Fatalf("parent status = %s", parent.Status)
	}
	tasks, err := svc.List(ListInput{})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(tasks) != 1 || tasks[0].Parent == nil || *tasks[0].Parent != parent.UUID {
		t.Fatalf("visible child not created: %#v", tasks)
	}
	firstChild := tasks[0]
	if err := svc.Done(firstChild.UUID); err != nil {
		t.Fatalf("Done(child) error = %v", err)
	}
	tasks, err = svc.List(ListInput{})
	if err != nil {
		t.Fatalf("List() after done error = %v", err)
	}
	if len(tasks) != 1 || tasks[0].UUID == firstChild.UUID {
		t.Fatalf("next child not generated: %#v", tasks)
	}
}

func TestServiceRecurringTaskPreservesAssignees(t *testing.T) {
	svc, closeFn := newTestService(t, mustUnix(t, "2030-01-01T10:00:00Z"))
	defer closeFn()
	due := mustUnix(t, "2030-01-01T23:59:59Z")
	until := mustUnix(t, "2030-02-01T23:59:59Z")
	recur := "daily"
	parent, err := svc.Add(AddInput{Description: "daily assigned task", Due: &due, Until: &until, Recur: &recur, Assignees: []string{"local"}})
	if err != nil {
		t.Fatalf("Add(recurring assigned) error = %v", err)
	}
	if len(parent.Assignees) != 1 || parent.Assignees[0].Name != "local" {
		t.Fatalf("parent assignees = %#v, want local", parent.Assignees)
	}
	tasks, err := svc.List(ListInput{})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(tasks) != 1 || len(tasks[0].Assignees) != 1 || tasks[0].Assignees[0].Name != "local" {
		t.Fatalf("first child assignees = %#v, want local", tasks)
	}
	firstChild := tasks[0]
	if err := svc.Done(firstChild.UUID); err != nil {
		t.Fatalf("Done(first child) error = %v", err)
	}
	tasks, err = svc.List(ListInput{})
	if err != nil {
		t.Fatalf("List() after done error = %v", err)
	}
	if len(tasks) != 1 || tasks[0].UUID == firstChild.UUID || len(tasks[0].Assignees) != 1 || tasks[0].Assignees[0].Name != "local" {
		t.Fatalf("next child assignees = %#v, want local", tasks)
	}
}

func TestServiceRecurringTaskRejectsMissingAssignee(t *testing.T) {
	svc, closeFn := newTestService(t, mustUnix(t, "2030-01-01T10:00:00Z"))
	defer closeFn()
	due := mustUnix(t, "2030-01-01T23:59:59Z")
	recur := "daily"
	_, err := svc.Add(AddInput{Description: "daily bad assignee", Due: &due, Recur: &recur, Assignees: []string{"missing-assignee"}})
	if err == nil {
		t.Fatal("Add(recurring missing assignee) error = nil, want assignee_not_found")
	}
	runtimeErr, ok := err.(RuntimeError)
	if !ok || runtimeErr.Code != "assignee_not_found" {
		t.Fatalf("Add(recurring missing assignee) err = %#v, want RuntimeError(assignee_not_found)", err)
	}
}

func TestRecurringStopsAtUntil(t *testing.T) {
	svc, closeFn := newTestService(t, mustUnix(t, "2030-01-01T10:00:00Z"))
	defer closeFn()
	due := mustUnix(t, "2030-01-01T23:59:59Z")
	until := due
	recur := "daily"
	if _, err := svc.Add(AddInput{Description: "daily", Due: &due, Until: &until, Recur: &recur}); err != nil {
		t.Fatal(err)
	}
	tasks, err := svc.List(ListInput{})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("children = %#v", tasks)
	}
	if err := svc.Done(tasks[0].UUID); err != nil {
		t.Fatal(err)
	}
	tasks, err = svc.List(ListInput{})
	if err != nil {
		t.Fatalf("List() after done error = %v", err)
	}
	if len(tasks) != 0 {
		t.Fatalf("child generated after until: %#v", tasks)
	}
}

func TestViewerListOnlyRefreshesCurrentWorkspaceWaitingTasks(t *testing.T) {
	store := newTestStore(t)
	ownerLocalCreate := newTestServiceWithRuntime(t, store, 100, "local", "local")
	waitLocal := int64(200)
	localWaiting, err := ownerLocalCreate.Add(AddInput{Description: "local waiting", Wait: &waitLocal})
	if err != nil {
		t.Fatalf("Add(local waiting) error = %v", err)
	}

	work, err := ownerLocalCreate.AddWorkspace(AddWorkspaceInput{Slug: "work", Name: "Work"})
	if err != nil {
		t.Fatalf("AddWorkspace(work) error = %v", err)
	}
	ownerWorkCreate := newTestServiceWithRuntime(t, store, 100, "local", work.Slug)
	waitWork := int64(200)
	if _, err := ownerWorkCreate.Add(AddInput{Description: "work waiting", Wait: &waitWork}); err != nil {
		t.Fatalf("Add(work waiting) error = %v", err)
	}

	viewer := mustCreateUserRecord(t, store, storage.User{ID: "user-viewer-scope", Name: "viewer-scope", CreatedAt: 100, ModifiedAt: 100})
	mustUpsertMembershipRecord(t, store, storage.Membership{
		UserID:      viewer.ID,
		WorkspaceID: work.ID,
		Role:        string(RoleViewer),
		JoinedAt:    100,
		ModifiedAt:  100,
	})

	viewerSvc := newTestServiceWithRuntime(t, store, 300, viewer.Name, work.Slug)
	tasks, err := viewerSvc.List(ListInput{})
	if err != nil {
		t.Fatalf("viewer List() error = %v", err)
	}
	if len(tasks) != 1 || tasks[0].Description != "work waiting" {
		t.Fatalf("viewer tasks = %#v", tasks)
	}

	ownerLocalRead := newTestServiceWithRuntime(t, store, 100, "local", "local")
	localTask, err := ownerLocalRead.Info(localWaiting.UUID)
	if err != nil {
		t.Fatalf("Info(local waiting) error = %v", err)
	}
	if localTask.Status != task.StatusWaiting {
		t.Fatalf("local task status = %s, want waiting", localTask.Status)
	}
}

func TestDoneRecurringTaskKeepsAuditAndNextChildInSameWorkspace(t *testing.T) {
	store := newTestStore(t)
	ownerLocal := newTestServiceWithRuntime(t, store, mustUnix(t, "2030-01-01T10:00:00Z"), "local", "local")
	work, err := ownerLocal.AddWorkspace(AddWorkspaceInput{Slug: "work", Name: "Work"})
	if err != nil {
		t.Fatalf("AddWorkspace(work) error = %v", err)
	}
	svc := newTestServiceWithRuntime(t, store, mustUnix(t, "2030-01-01T10:00:00Z"), "local", work.Slug)
	due := mustUnix(t, "2030-01-01T23:59:59Z")
	until := mustUnix(t, "2030-02-01T23:59:59Z")
	recur := "daily"
	parent, err := svc.Add(AddInput{Description: "daily work task", Due: &due, Until: &until, Recur: &recur})
	if err != nil {
		t.Fatalf("Add(recurring) error = %v", err)
	}
	tasks, err := svc.List(ListInput{})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("tasks = %#v", tasks)
	}
	firstChild := tasks[0]
	if err := svc.Done(firstChild.UUID); err != nil {
		t.Fatalf("Done(child) error = %v", err)
	}

	logs, err := svc.ListAudit(AuditListInput{Limit: 10})
	if err != nil {
		t.Fatalf("ListAudit() error = %v", err)
	}
	if len(logs) == 0 || logs[0].Action != "task.done" || logs[0].TargetID != firstChild.UUID {
		t.Fatalf("logs = %#v", logs)
	}

	tasks, err = svc.List(ListInput{})
	if err != nil {
		t.Fatalf("List() after Done error = %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("tasks after Done = %#v", tasks)
	}
	nextChild := tasks[0]
	if nextChild.UUID == firstChild.UUID {
		t.Fatalf("next child not created: %#v", tasks)
	}
	if nextChild.WorkspaceID != work.ID || nextChild.Parent == nil || *nextChild.Parent != parent.UUID {
		t.Fatalf("next child = %#v", nextChild)
	}
}

func TestProjectConfigAgentKeysAndSizeLimit(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()
	project, err := svc.AddProject(AddProjectInput{Slug: "agent", Name: "Agent"})
	if err != nil {
		t.Fatalf("AddProject() error = %v", err)
	}
	for _, key := range []string{"agent.background", "agent.constraints", "agent.default_context", "agent.handoff"} {
		if err := svc.ProjectConfigSet(project.Slug, key, "ok"); err != nil {
			t.Fatalf("ProjectConfigSet(%s) error = %v", key, err)
		}
		if value, ok, err := svc.ProjectConfigGet(project.Slug, key); err != nil || !ok || value != "ok" {
			t.Fatalf("ProjectConfigGet(%s) = %q,%v,%v", key, value, ok, err)
		}
	}
	err = svc.ProjectConfigSet(project.Slug, "agent.background", strings.Repeat("x", agentConfigValueMaxBytes+1))
	assertRuntimeCode(t, err, "config_value_too_large")
}

func containsTask(tasks []task.Task, uuid string) bool {
	for _, tsk := range tasks {
		if tsk.UUID == uuid {
			return true
		}
	}
	return false
}

func hasUrgencyItem(explain urgency.ExplainResult, name string) bool {
	for _, item := range explain.Items {
		if item.Name == name {
			return true
		}
	}
	return false
}

func mustUnix(t *testing.T, value string) int64 {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatalf("time.Parse(%q) error = %v", value, err)
	}
	return parsed.Unix()
}

func TestServiceTaskAddLink(t *testing.T) {
	store := newTestStore(t)
	svc := newTestServiceWithRuntime(t, store, 100, "local", "local")

	created, err := svc.Add(AddInput{Description: "test task"})
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	info, err := svc.TaskAddLink(created.UUID, "document", "https://example.com/doc", "Test Doc")
	if err != nil {
		t.Fatalf("TaskAddLink() error = %v", err)
	}
	if info.Type != "document" {
		t.Fatalf("Type = %q, want %q", info.Type, "document")
	}
	if info.URL != "https://example.com/doc" {
		t.Fatalf("URL = %q, want %q", info.URL, "https://example.com/doc")
	}
	if info.Title != "Test Doc" {
		t.Fatalf("Title = %q, want %q", info.Title, "Test Doc")
	}

	tsk, err := svc.Info(created.UUID)
	if err != nil {
		t.Fatalf("Info() error = %v", err)
	}
	if len(tsk.Links) != 1 {
		t.Fatalf("Links count = %d, want 1", len(tsk.Links))
	}
	if tsk.Links[0].ID != info.ID {
		t.Fatalf("Link ID = %q, want %q", tsk.Links[0].ID, info.ID)
	}
}

func TestServiceTaskAddLinkDuplicateURL(t *testing.T) {
	store := newTestStore(t)
	svc := newTestServiceWithRuntime(t, store, 100, "local", "local")

	created, err := svc.Add(AddInput{Description: "test task"})
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	_, err = svc.TaskAddLink(created.UUID, "document", "https://example.com/doc", "First")
	if err != nil {
		t.Fatalf("first TaskAddLink() error = %v", err)
	}

	_, err = svc.TaskAddLink(created.UUID, "document", "https://example.com/doc", "Second")
	if err == nil {
		t.Fatal("duplicate TaskAddLink() error = nil, want link_duplicate")
	}
	runtimeErr, ok := err.(RuntimeError)
	if !ok || (runtimeErr.Code != "link_duplicate" && !strings.Contains(runtimeErr.Message, "already linked")) {
		t.Fatalf("duplicate TaskAddLink() err = %#v, want RuntimeError with link_duplicate", err)
	}
}

func TestServiceTaskRemoveLink(t *testing.T) {
	store := newTestStore(t)
	svc := newTestServiceWithRuntime(t, store, 100, "local", "local")

	created, err := svc.Add(AddInput{Description: "test task"})
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	linkInfo, err := svc.TaskAddLink(created.UUID, "document", "https://example.com/doc", "Test Doc")
	if err != nil {
		t.Fatalf("TaskAddLink() error = %v", err)
	}

	if err := svc.TaskRemoveLink(created.UUID, linkInfo.ID); err != nil {
		t.Fatalf("TaskRemoveLink() error = %v", err)
	}

	tsk, err := svc.Info(created.UUID)
	if err != nil {
		t.Fatalf("Info() error = %v", err)
	}
	if len(tsk.Links) != 0 {
		t.Fatalf("Links after remove = %d, want 0", len(tsk.Links))
	}
}

func TestServiceTaskRemoveLinkNotFound(t *testing.T) {
	store := newTestStore(t)
	svc := newTestServiceWithRuntime(t, store, 100, "local", "local")

	created, err := svc.Add(AddInput{Description: "test task"})
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	err = svc.TaskRemoveLink(created.UUID, "nonexistent-link-id")
	if err == nil {
		t.Fatal("TaskRemoveLink(nonexistent) error = nil, want link_not_found")
	}
	runtimeErr, ok := err.(RuntimeError)
	if !ok || runtimeErr.Code != "link_not_found" {
		t.Fatalf("TaskRemoveLink(nonexistent) err = %#v, want RuntimeError(link_not_found)", err)
	}
}

func TestServiceTaskRemoveLinkWrongTask(t *testing.T) {
	store := newTestStore(t)
	svc := newTestServiceWithRuntime(t, store, 100, "local", "local")

	task1, err := svc.Add(AddInput{Description: "task one"})
	if err != nil {
		t.Fatalf("Add(task1) error = %v", err)
	}
	task2, err := svc.Add(AddInput{Description: "task two"})
	if err != nil {
		t.Fatalf("Add(task2) error = %v", err)
	}

	linkInfo, err := svc.TaskAddLink(task1.UUID, "document", "https://example.com/doc", "Test Doc")
	if err != nil {
		t.Fatalf("TaskAddLink() error = %v", err)
	}

	err = svc.TaskRemoveLink(task2.UUID, linkInfo.ID)
	if err == nil {
		t.Fatal("TaskRemoveLink(wrong task) error = nil, want link_not_found")
	}
	runtimeErr, ok := err.(RuntimeError)
	if !ok || runtimeErr.Code != "link_not_found" {
		t.Fatalf("TaskRemoveLink(wrong task) err = %#v, want RuntimeError(link_not_found)", err)
	}
}

func TestServiceTaskAddLinkRejectsCompletedTask(t *testing.T) {
	store := newTestStore(t)
	svc := newTestServiceWithRuntime(t, store, 100, "local", "local")

	created, err := svc.Add(AddInput{Description: "test task"})
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	if err := svc.Done(created.UUID); err != nil {
		t.Fatalf("Done() error = %v", err)
	}

	_, err = svc.TaskAddLink(created.UUID, "document", "https://example.com/doc", "Test Doc")
	if err == nil {
		t.Fatal("TaskAddLink(completed) error = nil, want task_not_writable")
	}
	runtimeErr, ok := err.(RuntimeError)
	if !ok || runtimeErr.Code != "task_not_writable" {
		t.Fatalf("TaskAddLink(completed) err = %#v, want RuntimeError(task_not_writable)", err)
	}
}

func TestServiceTaskAddLinkUpdatesModifiedTimestamp(t *testing.T) {
	store := newTestStore(t)
	svc := newTestServiceWithRuntime(t, store, 100, "local", "local")

	created, err := svc.Add(AddInput{Description: "test task"})
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	beforeModify := created.Modified

	svc.clock = FixedClock{NowUnix: 200}
	if _, err := svc.TaskAddLink(created.UUID, "document", "https://example.com/doc", "Test Doc"); err != nil {
		t.Fatalf("TaskAddLink() error = %v", err)
	}

	tsk, err := svc.Info(created.UUID)
	if err != nil {
		t.Fatalf("Info() error = %v", err)
	}
	if tsk.Modified <= beforeModify {
		t.Fatalf("Modified = %d, want > %d", tsk.Modified, beforeModify)
	}
}

func TestServiceTaskAddLinkRequiresTypeAndURL(t *testing.T) {
	store := newTestStore(t)
	svc := newTestServiceWithRuntime(t, store, 100, "local", "local")

	created, err := svc.Add(AddInput{Description: "test task"})
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	_, err = svc.TaskAddLink(created.UUID, "", "https://example.com/doc", "Test Doc")
	if err == nil {
		t.Fatal("TaskAddLink(empty type) error = nil, want link_type_required")
	}
	runtimeErr, ok := err.(RuntimeError)
	if !ok || runtimeErr.Code != "link_type_required" {
		t.Fatalf("TaskAddLink(empty type) err = %#v, want RuntimeError(link_type_required)", err)
	}

	_, err = svc.TaskAddLink(created.UUID, "document", "", "Test Doc")
	if err == nil {
		t.Fatal("TaskAddLink(empty url) error = nil, want link_url_required")
	}
	runtimeErr, ok = err.(RuntimeError)
	if !ok || runtimeErr.Code != "link_url_required" {
		t.Fatalf("TaskAddLink(empty url) err = %#v, want RuntimeError(link_url_required)", err)
	}
}

func TestAddProjectRejectsDigitStartSlug(t *testing.T) {
	store := newTestStore(t)
	svc := newTestServiceWithRuntime(t, store, 100, "local", "local")
	_, err := svc.AddProject(AddProjectInput{Slug: "123project", Name: "Test"})
	if err == nil {
		t.Fatal("expected digit-starting slug to be rejected")
	}
}

func TestServiceProjectAnnotateAndList(t *testing.T) {
	store := newTestStore(t)
	svc := newTestServiceWithRuntime(t, store, 100, "local", "local")

	project, err := svc.AddProject(AddProjectInput{Slug: "test-proj", Name: "Test"})
	if err != nil {
		t.Fatal(err)
	}

	annotation, err := svc.ProjectAnnotate("test-proj", "first note")
	if err != nil {
		t.Fatal(err)
	}
	if annotation.Content != "first note" {
		t.Fatalf("Content = %q, want %q", annotation.Content, "first note")
	}

	annotations, err := svc.ProjectAnnotations("test-proj")
	if err != nil {
		t.Fatal(err)
	}
	if len(annotations) != 1 {
		t.Fatalf("annotations count = %d, want 1", len(annotations))
	}
	if annotations[0].ID != annotation.ID {
		t.Fatalf("annotation ID mismatch")
	}

	_ = project
}

func TestServiceProjectDenotate(t *testing.T) {
	store := newTestStore(t)
	svc := newTestServiceWithRuntime(t, store, 100, "local", "local")

	_, err := svc.AddProject(AddProjectInput{Slug: "test-proj", Name: "Test"})
	if err != nil {
		t.Fatal(err)
	}

	annotation, err := svc.ProjectAnnotate("test-proj", "will be removed")
	if err != nil {
		t.Fatal(err)
	}

	if err := svc.ProjectDenotate("test-proj", annotation.ID); err != nil {
		t.Fatalf("ProjectDenotate() error = %v", err)
	}

	annotations, err := svc.ProjectAnnotations("test-proj")
	if err != nil {
		t.Fatal(err)
	}
	if len(annotations) != 0 {
		t.Fatalf("annotations after denotate = %d, want 0", len(annotations))
	}
}

func TestServiceProjectDenotateNotFound(t *testing.T) {
	store := newTestStore(t)
	svc := newTestServiceWithRuntime(t, store, 100, "local", "local")

	_, err := svc.AddProject(AddProjectInput{Slug: "test-proj", Name: "Test"})
	if err != nil {
		t.Fatal(err)
	}

	err = svc.ProjectDenotate("test-proj", "nonexistent-id")
	if err == nil {
		t.Fatal("ProjectDenotate(nonexistent) error = nil, want error")
	}
}

func TestServiceProjectAnnotateRejectsArchived(t *testing.T) {
	store := newTestStore(t)
	svc := newTestServiceWithRuntime(t, store, 100, "local", "local")

	project, err := svc.AddProject(AddProjectInput{Slug: "test-proj", Name: "Test"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ArchiveProject(project.ID); err != nil {
		t.Fatalf("ArchiveProject() error = %v", err)
	}

	_, err = svc.ProjectAnnotate("test-proj", "should fail")
	if err == nil {
		t.Fatal("ProjectAnnotate(archived) error = nil, want error")
	}
}

func TestServiceProjectAnnotateRejectsEmptyContent(t *testing.T) {
	store := newTestStore(t)
	svc := newTestServiceWithRuntime(t, store, 100, "local", "local")

	_, err := svc.AddProject(AddProjectInput{Slug: "test-proj", Name: "Test"})
	if err != nil {
		t.Fatal(err)
	}

	_, err = svc.ProjectAnnotate("test-proj", "")
	if err == nil {
		t.Fatal("ProjectAnnotate(empty) error = nil, want error")
	}
}

func TestServiceProjectAnnotateUpdatesModifiedAt(t *testing.T) {
	store := newTestStore(t)
	svc := newTestServiceWithRuntime(t, store, 100, "local", "local")

	project, err := svc.AddProject(AddProjectInput{Slug: "test-proj", Name: "Test"})
	if err != nil {
		t.Fatal(err)
	}
	beforeModified := project.ModifiedAt

	svc2 := newTestServiceWithRuntime(t, store, 200, "local", "local")
	_, err = svc2.ProjectAnnotate("test-proj", "new note")
	if err != nil {
		t.Fatal(err)
	}

	updated, err := svc2.ProjectInfo("test-proj")
	if err != nil {
		t.Fatal(err)
	}
	if updated.ModifiedAt <= beforeModified {
		t.Fatalf("ModifiedAt = %d, want > %d", updated.ModifiedAt, beforeModified)
	}
}

func TestServiceProjectTimeline(t *testing.T) {
	store := newTestStore(t)
	svc := newTestServiceWithRuntime(t, store, 100, "local", "local")

	project, err := svc.AddProject(AddProjectInput{Slug: "test-proj", Name: "Test"})
	if err != nil {
		t.Fatal(err)
	}

	_, err = svc.ProjectAnnotate("test-proj", "project note")
	if err != nil {
		t.Fatal(err)
	}

	task, err := svc.Add(AddInput{Description: "task", Project: &project.Slug})
	if err != nil {
		t.Fatal(err)
	}
	svc2 := newTestServiceWithRuntime(t, store, 200, "local", "local")
	if err := svc2.Annotate(task.UUID, "task note"); err != nil {
		t.Fatal(err)
	}

	entries, err := svc2.ProjectTimeline("test-proj", TimelineOptions{Limit: 50, Offset: 0})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("timeline entries = %d, want 2", len(entries))
	}

	sourceTypes := map[string]bool{}
	for _, e := range entries {
		sourceTypes[e.SourceType] = true
	}
	if !sourceTypes["project"] || !sourceTypes["task"] {
		t.Fatalf("timeline missing source types: %+v", entries)
	}
}

func TestServiceProjectAnnotateTimestampConflict(t *testing.T) {
	store := newTestStore(t)
	svc := newTestServiceWithRuntime(t, store, 100, "local", "local")

	_, err := svc.AddProject(AddProjectInput{Slug: "test-proj", Name: "Test"})
	if err != nil {
		t.Fatal(err)
	}

	a1, err := svc.ProjectAnnotate("test-proj", "first at timestamp 100")
	if err != nil {
		t.Fatal(err)
	}

	svc2 := newTestServiceWithRuntime(t, store, 100, "local", "local")
	a2, err := svc2.ProjectAnnotate("test-proj", "second at timestamp 100")
	if err != nil {
		t.Fatalf("ProjectAnnotate with same timestamp: error = %v", err)
	}
	if a1.ID == a2.ID {
		t.Fatal("second annotation should have different ID")
	}
	if a1.Entry == a2.Entry {
		t.Fatalf("entry should differ on conflict: a1.Entry=%d, a2.Entry=%d", a1.Entry, a2.Entry)
	}

	annotations, err := svc.ProjectAnnotations("test-proj")
	if err != nil {
		t.Fatal(err)
	}
	if len(annotations) != 2 {
		t.Fatalf("annotations count = %d, want 2", len(annotations))
	}
}

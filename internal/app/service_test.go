package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dajee/taskg/internal/storage/sqlite"
	"github.com/dajee/taskg/internal/task"
	taskrcparser "github.com/dajee/taskg/internal/taskrc"
	"github.com/dajee/taskg/internal/urgency"
)

func strptr(v string) *string { return &v }

func newTestService(t *testing.T, now int64) (*Service, func()) {
	t.Helper()
	store, err := sqlite.Open(filepath.Join(t.TempDir(), "taskg.db"))
	if err != nil {
		t.Fatal(err)
	}
	svc, err := NewService(ServiceOptions{Store: store, Clock: fixedClock{NowUnix: now}})
	if err != nil {
		t.Fatal(err)
	}
	return svc, func() { _ = store.Close() }
}

func newTestStore(t *testing.T) *sqlite.Store {
	t.Helper()
	store, err := sqlite.Open(filepath.Join(t.TempDir(), "taskg.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func newTestServiceWithRuntime(t *testing.T, store *sqlite.Store, now int64, actorRef, workspaceRef string) *Service {
	t.Helper()
	svc, err := NewService(ServiceOptions{
		Store:        store,
		Clock:        fixedClock{NowUnix: now},
		ActorRef:     actorRef,
		WorkspaceRef: workspaceRef,
	})
	if err != nil {
		t.Fatal(err)
	}
	return svc
}

func TestServiceAddListInfo(t *testing.T) {
	store, err := sqlite.Open(filepath.Join(t.TempDir(), "taskg.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	svc, err := NewService(ServiceOptions{Store: store, Clock: fixedClock{NowUnix: 100}})
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
	userRepo := sqlite.NewUserRepository(store.DB())
	wsRepo := sqlite.NewWorkspaceRepository(store.DB())
	memberRepo := sqlite.NewMemberRepository(store.DB())

	localUser, err := userRepo.GetByName("local")
	if err != nil {
		t.Fatalf("GetByName(local) error = %v", err)
	}
	work, err := wsRepo.Create(sqlite.Workspace{
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
	if err := memberRepo.Upsert(sqlite.Membership{
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
		Clock:        fixedClock{NowUnix: 100},
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

	userRepo := sqlite.NewUserRepository(store.DB())
	memberRepo := sqlite.NewMemberRepository(store.DB())
	ws, err := store.LocalWorkspace()
	if err != nil {
		t.Fatalf("LocalWorkspace() error = %v", err)
	}
	viewer, err := userRepo.Create(sqlite.User{ID: "user-viewer", Name: "viewer", CreatedAt: 100, ModifiedAt: 100})
	if err != nil {
		t.Fatalf("Create(viewer) error = %v", err)
	}
	if err := memberRepo.Upsert(sqlite.Membership{
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

func TestViewerCanUseOwnContextButCannotDefineContext(t *testing.T) {
	store := newTestStore(t)
	ownerSvc := newTestServiceWithRuntime(t, store, 100, "local", "local")
	if err := ownerSvc.DefineContext("work", "project:work"); err != nil {
		t.Fatalf("DefineContext(owner) error = %v", err)
	}

	userRepo := sqlite.NewUserRepository(store.DB())
	memberRepo := sqlite.NewMemberRepository(store.DB())
	ws, err := store.LocalWorkspace()
	if err != nil {
		t.Fatalf("LocalWorkspace() error = %v", err)
	}
	viewer, err := userRepo.Create(sqlite.User{ID: "user-viewer-ctx", Name: "viewer-ctx", CreatedAt: 100, ModifiedAt: 100})
	if err != nil {
		t.Fatalf("Create(viewer) error = %v", err)
	}
	if err := memberRepo.Upsert(sqlite.Membership{
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
	userRepo := sqlite.NewUserRepository(store.DB())
	memberRepo := sqlite.NewMemberRepository(store.DB())
	ws, err := store.LocalWorkspace()
	if err != nil {
		t.Fatalf("LocalWorkspace() error = %v", err)
	}
	viewer, err := userRepo.Create(sqlite.User{ID: "user-viewer-uda", Name: "viewer-uda", CreatedAt: 100, ModifiedAt: 100})
	if err != nil {
		t.Fatalf("Create(viewer) error = %v", err)
	}
	if err := memberRepo.Upsert(sqlite.Membership{
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
	userRepo := sqlite.NewUserRepository(store.DB())
	memberRepo := sqlite.NewMemberRepository(store.DB())
	ws, err := store.LocalWorkspace()
	if err != nil {
		t.Fatalf("LocalWorkspace() error = %v", err)
	}
	member, err := userRepo.Create(sqlite.User{ID: "user-member", Name: "member-user", CreatedAt: 100, ModifiedAt: 100})
	if err != nil {
		t.Fatalf("Create(member) error = %v", err)
	}
	if err := memberRepo.Upsert(sqlite.Membership{
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
	userRepo := sqlite.NewUserRepository(store.DB())
	memberRepo := sqlite.NewMemberRepository(store.DB())
	ws, err := store.LocalWorkspace()
	if err != nil {
		t.Fatalf("LocalWorkspace() error = %v", err)
	}
	admin, err := userRepo.Create(sqlite.User{ID: "user-admin", Name: "admin-user", CreatedAt: 100, ModifiedAt: 100})
	if err != nil {
		t.Fatalf("Create(admin) error = %v", err)
	}
	if err := memberRepo.Upsert(sqlite.Membership{
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

type failingAuditRepo struct {
	listRows []sqlite.AuditLogEntry
}

func (f *failingAuditRepo) Append(sqlite.AuditLogEntry) error {
	return sqlite.ErrNotFound
}

func (f *failingAuditRepo) List(sqlite.AuditListOptions) ([]sqlite.AuditLogEntry, error) {
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
	store, err := sqlite.Open(filepath.Join(t.TempDir(), "taskg.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	svc, err := NewService(ServiceOptions{
		Store:         store,
		Clock:         fixedClock{NowUnix: 100},
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
	store, err := sqlite.Open(filepath.Join(t.TempDir(), "taskg.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	svc, err := NewService(ServiceOptions{
		Store: store,
		Clock: fixedClock{NowUnix: 100},
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
	store, err := sqlite.Open(filepath.Join(t.TempDir(), "taskg.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	svc, err := NewService(ServiceOptions{
		Store: store,
		Clock: fixedClock{NowUnix: 100},
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
	store, err := sqlite.Open(filepath.Join(t.TempDir(), "taskg.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	svc, err := NewService(ServiceOptions{
		Store:         store,
		Clock:         fixedClock{NowUnix: 100},
		RuntimeConfig: map[string]string{"context.active": "work"},
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	if err := svc.DefineContext("work", "project:work"); err != nil {
		t.Fatalf("DefineContext() error = %v", err)
	}
	if show, err := svc.ContextShow(); err != nil || !strings.Contains(show, "work") {
		t.Fatalf("ContextShow() with runtime config = %q, %v", show, err)
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

func TestNoContextBypassesActiveContext(t *testing.T) {
	svc, closeFn := newTestService(t, 100)
	defer closeFn()
	_, _ = svc.Add(AddInput{Description: "work task", Project: strptr("work")})
	_, _ = svc.Add(AddInput{Description: "home task", Project: strptr("home")})
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

func TestServiceModifyDoneDeleteByNumber(t *testing.T) {
	store, err := sqlite.Open(filepath.Join(t.TempDir(), "taskg.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	svc, _ := NewService(ServiceOptions{Store: store, Clock: fixedClock{NowUnix: 100}})

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

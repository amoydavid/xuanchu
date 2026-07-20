package app

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"

	"git.dajee.net/dajee/xuanchu/internal/attachments"
	"git.dajee.net/dajee/xuanchu/internal/storage"
)

func attachmentTestEnv(t *testing.T) (*Service, func()) {
	t.Helper()
	store, err := storage.Open(filepath.Join(t.TempDir(), "xuanchu.db"))
	if err != nil {
		t.Fatal(err)
	}
	rt, err := newAttachmentTestRuntime(t)
	if err != nil {
		t.Fatal(err)
	}
	svc, err := NewService(ServiceOptions{
		Store:       store,
		Clock:       FixedClock{NowUnix: 1000},
		Attachments: rt,
	})
	if err != nil {
		t.Fatal(err)
	}
	return svc, func() { _ = store.Close() }
}

func newAttachmentTestRuntime(t *testing.T) (*AttachmentRuntime, error) {
	t.Helper()
	cfg := attachments.DefaultConfig(t.TempDir())
	cfg.MaxResourceTotalSizeBytes = 1 << 20
	cfg.MaxWorkspaceTotalSizeBytes = 2 << 20
	cfg.MaxAttachmentsPerResource = 10
	cfg.MaxFileSizeBytes = 256 * 1024
	return NewAttachmentRuntime(context.Background(), cfg)
}

func pngBytes2x2() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 255})
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

func TestAttachmentUploadAndList(t *testing.T) {
	svc, closeFn := attachmentTestEnv(t)
	defer closeFn()
	tsk, err := svc.Add(AddInput{Title: "with-attachment"})
	if err != nil {
		t.Fatal(err)
	}

	payload := pngBytes2x2()
	view, err := svc.UploadAttachment(context.Background(), "task", tsk.UUID, AttachmentUploadInput{
		Reader:       bytes.NewReader(payload),
		DeclaredSize: int64(len(payload)),
		OriginalName: "diagram.png",
		Mode:         "attachment",
	})
	if err != nil {
		t.Fatalf("UploadAttachment: %v", err)
	}
	if view.State != "active" {
		t.Fatalf("state = %q", view.State)
	}
	if view.InlineCapable != true || view.MediaType != "image/png" {
		t.Fatalf("view = %#v", view)
	}
	if view.AttachedTo.Type != "task" || view.AttachedTo.ID != tsk.UUID {
		t.Fatalf("attached_to = %#v", view.AttachedTo)
	}
	if view.CreatedBy.Type != "user" || view.CreatedBy.User == nil {
		t.Fatalf("created_by = %#v", view.CreatedBy)
	}

	list, err := svc.ListAttachments("task", tsk.UUID, false)
	if err != nil {
		t.Fatalf("ListAttachments: %v", err)
	}
	if len(list) != 1 || list[0].ID != view.ID {
		t.Fatalf("list = %#v", list)
	}
}

func TestAttachmentUploadDraftMode(t *testing.T) {
	svc, closeFn := attachmentTestEnv(t)
	defer closeFn()
	tsk, _ := svc.Add(AddInput{Title: "draft-task"})

	payload := pngBytes2x2()
	view, err := svc.UploadAttachment(context.Background(), "task", tsk.UUID, AttachmentUploadInput{
		Reader:       bytes.NewReader(payload),
		DeclaredSize: int64(len(payload)),
		OriginalName: "draft.png",
		Mode:         "description_draft",
	})
	if err != nil {
		t.Fatalf("UploadAttachment: %v", err)
	}
	if view.State != "draft" {
		t.Fatalf("state = %q", view.State)
	}
	// 不 include_drafts 时 draft 不在普通列表。
	list, _ := svc.ListAttachments("task", tsk.UUID, false)
	if len(list) != 0 {
		t.Fatalf("draft leaked into normal list: %#v", list)
	}
	// include_drafts 时由创建者可见。
	withDrafts, _ := svc.ListAttachments("task", tsk.UUID, true)
	if len(withDrafts) != 1 {
		t.Fatalf("draft not visible to creator: %#v", withDrafts)
	}
}

func TestTaskDescriptionDraftIsOnlyAccessibleToCreator(t *testing.T) {
	svc, closeFn := attachmentTestEnv(t)
	defer closeFn()
	tsk, err := svc.Add(AddInput{Title: "draft privacy"})
	if err != nil {
		t.Fatal(err)
	}
	payload := pngBytes2x2()
	draft, err := svc.UploadAttachment(context.Background(), "task", tsk.UUID, AttachmentUploadInput{
		Reader:       bytes.NewReader(payload),
		DeclaredSize: int64(len(payload)),
		OriginalName: "private.png",
		Mode:         "description_draft",
	})
	if err != nil {
		t.Fatal(err)
	}
	viewerUser := storage.User{ID: uuid.NewString(), Name: "draft-viewer", DisplayName: "Draft Viewer", CreatedAt: 1000, ModifiedAt: 1000}
	if _, err := storage.NewUserRepository(svc.store.DB()).Create(viewerUser); err != nil {
		t.Fatal(err)
	}
	if err := storage.NewMemberRepository(svc.store.DB()).Upsert(storage.Membership{UserID: viewerUser.ID, WorkspaceID: svc.Runtime().WorkspaceID, Role: "member", JoinedAt: 1000, ModifiedAt: 1000}); err != nil {
		t.Fatal(err)
	}
	viewer, err := NewService(ServiceOptions{
		Store:        svc.store,
		Clock:        FixedClock{NowUnix: 1000},
		ActorRef:     viewerUser.Name,
		WorkspaceRef: "local",
		Attachments:  svc.attachmentRuntime,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := viewer.GetAttachment(draft.ID); err == nil {
		t.Fatal("GetAttachment exposed another actor's draft")
	} else {
		assertRuntimeCode(t, err, "attachment_not_found")
	}
	if _, err := viewer.OpenAttachmentContent(context.Background(), draft.ID); err == nil {
		t.Fatal("OpenAttachmentContent exposed another actor's draft")
	} else {
		assertRuntimeCode(t, err, "attachment_not_found")
	}
	if _, err := viewer.RenameAttachment(draft.ID, "stolen"); err == nil {
		t.Fatal("RenameAttachment exposed another actor's draft")
	} else {
		assertRuntimeCode(t, err, "attachment_not_found")
	}
	if err := viewer.RemoveAttachment(context.Background(), draft.ID); err == nil {
		t.Fatal("RemoveAttachment exposed another actor's draft")
	} else {
		assertRuntimeCode(t, err, "attachment_not_found")
	}
}

func TestAddRejectsDescriptionAttachmentFromAnotherTask(t *testing.T) {
	svc, closeFn := attachmentTestEnv(t)
	defer closeFn()
	owner, _ := svc.Add(AddInput{Title: "attachment-owner"})
	payload := pngBytes2x2()
	attachment, err := svc.UploadAttachment(context.Background(), "task", owner.UUID, AttachmentUploadInput{
		Reader: bytes.NewReader(payload), DeclaredSize: int64(len(payload)), OriginalName: "draft.png", Mode: "description_draft",
	})
	if err != nil {
		t.Fatal(err)
	}
	description := fmt.Sprintf("![图](ref://attachment/%s)", attachment.ID)
	_, err = svc.Add(AddInput{Title: "foreign-reference", Description: &description})
	assertRuntimeCode(t, err, "description_reference_invalid")
}

func TestModifyRejectsDescriptionDraftCreatedByAnotherActor(t *testing.T) {
	svc, closeFn := attachmentTestEnv(t)
	defer closeFn()
	tsk, err := svc.Add(AddInput{Title: "draft owner boundary"})
	if err != nil {
		t.Fatal(err)
	}
	payload := pngBytes2x2()
	draft, err := svc.UploadAttachment(context.Background(), "task", tsk.UUID, AttachmentUploadInput{
		Reader: bytes.NewReader(payload), DeclaredSize: int64(len(payload)), OriginalName: "private.png", Mode: "description_draft",
	})
	if err != nil {
		t.Fatal(err)
	}
	other := storage.User{ID: uuid.NewString(), Name: "other-editor", DisplayName: "Other Editor", CreatedAt: 1000, ModifiedAt: 1000}
	if _, err := storage.NewUserRepository(svc.store.DB()).Create(other); err != nil {
		t.Fatal(err)
	}
	if err := storage.NewMemberRepository(svc.store.DB()).Upsert(storage.Membership{
		UserID: other.ID, WorkspaceID: svc.Runtime().WorkspaceID, Role: "member", JoinedAt: 1000, ModifiedAt: 1000,
	}); err != nil {
		t.Fatal(err)
	}
	otherSvc, err := NewService(ServiceOptions{
		Store: svc.store, Clock: FixedClock{NowUnix: 1000}, ActorRef: other.Name,
		WorkspaceRef: "local", Attachments: svc.attachmentRuntime,
	})
	if err != nil {
		t.Fatal(err)
	}
	description := fmt.Sprintf("![private](ref://attachment/%s)", draft.ID)
	err = otherSvc.Modify(tsk.UUID, ModifyInput{Description: &description})
	assertRuntimeCode(t, err, "attachment_draft_creator_mismatch")
}

func TestAddRollsBackTaskAndDraftBindingWhenDescriptionValidationFails(t *testing.T) {
	svc, closeFn := attachmentTestEnv(t)
	defer closeFn()
	draftTarget := uuid.NewString()
	payload := pngBytes2x2()
	draft, err := svc.UploadAttachment(context.Background(), "task_draft", draftTarget, AttachmentUploadInput{
		Reader:       bytes.NewReader(payload),
		DeclaredSize: int64(len(payload)),
		OriginalName: "draft.png",
		Mode:         "description_draft",
	})
	if err != nil {
		t.Fatalf("upload task draft: %v", err)
	}
	// 该引用不属于本次创建的 draft target，触发 Add 中的 description 校验失败。
	foreignID := uuid.NewString()
	description := fmt.Sprintf("![图](ref://attachment/%s)", foreignID)
	_, err = svc.Add(AddInput{
		Title:                 "must rollback",
		Description:           &description,
		AttachmentDraftTarget: draftTarget,
	})
	assertRuntimeCode(t, err, "description_reference_invalid")

	list, err := svc.List(ListInput{})
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("failed creation persisted a task: %#v", list)
	}
	row, err := svc.attachmentRepo.GetByID(draft.ID)
	if err != nil {
		t.Fatalf("get draft: %v", err)
	}
	if row.AttachedToType != storage.AttachmentAttachedToTaskDraft || row.AttachedToID != draftTarget || row.State != storage.AttachmentStateDraft {
		t.Fatalf("failed creation rebound draft: %#v", row)
	}
}

func TestAddBindsOnlyDescriptionReferencedCreationDrafts(t *testing.T) {
	svc, closeFn := attachmentTestEnv(t)
	defer closeFn()
	draftTarget := uuid.NewString()
	payload := pngBytes2x2()
	used, err := svc.UploadAttachment(context.Background(), "task_draft", draftTarget, AttachmentUploadInput{
		Reader: bytes.NewReader(payload), DeclaredSize: int64(len(payload)), OriginalName: "used.png", Mode: "description_draft",
	})
	if err != nil {
		t.Fatal(err)
	}
	unused, err := svc.UploadAttachment(context.Background(), "task_draft", draftTarget, AttachmentUploadInput{
		Reader: bytes.NewReader(payload), DeclaredSize: int64(len(payload)), OriginalName: "unused.png", Mode: "description_draft",
	})
	if err != nil {
		t.Fatal(err)
	}
	description := fmt.Sprintf("![used](ref://attachment/%s)", used.ID)
	created, err := svc.Add(AddInput{
		Title: "only bind referenced draft", Description: &description, AttachmentDraftTarget: draftTarget,
	})
	if err != nil {
		t.Fatal(err)
	}
	usedRow, err := svc.attachmentRepo.GetByID(used.ID)
	if err != nil {
		t.Fatal(err)
	}
	if usedRow.AttachedToType != storage.AttachmentAttachedToTask || usedRow.AttachedToID != created.UUID || usedRow.State != storage.AttachmentStateActive {
		t.Fatalf("referenced draft not rebound and activated: %#v", usedRow)
	}
	unusedRow, err := svc.attachmentRepo.GetByID(unused.ID)
	if err != nil {
		t.Fatal(err)
	}
	if unusedRow.AttachedToType != storage.AttachmentAttachedToTaskDraft || unusedRow.AttachedToID != draftTarget || unusedRow.State != storage.AttachmentStateDraft {
		t.Fatalf("unreferenced draft was rebound: %#v", unusedRow)
	}
}

func TestAddEnforcesDescriptionSizeWithoutAttachmentRuntime(t *testing.T) {
	svc, closeFn := newTestService(t, 1000)
	defer closeFn()
	description := strings.Repeat("x", maxDescriptionBytes+1)
	_, err := svc.Add(AddInput{Title: "oversized-description", Description: &description})
	assertRuntimeCode(t, err, "description_too_large")
}

func TestAddRejectsUnavailableUserReference(t *testing.T) {
	svc, closeFn := newTestService(t, 1000)
	defer closeFn()
	description := "[@陌生人](ref://user/40af0185-316f-42bb-b52b-545d21f6f012)"
	_, err := svc.Add(AddInput{Title: "unavailable-user-reference", Description: &description})
	assertRuntimeCode(t, err, "description_reference_invalid")
}

func TestModifyAllowsUnchangedUnavailableUserReference(t *testing.T) {
	svc, closeFn := newTestService(t, 1000)
	defer closeFn()
	userID := svc.Runtime().ActorUserID
	description := fmt.Sprintf("[@当前用户](ref://user/%s)", userID)
	created, err := svc.Add(AddInput{Title: "historical-reference", Description: &description})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := svc.store.DB().Where("user_id = ? AND workspace_id = ?", userID, svc.Runtime().WorkspaceID).Delete(&storage.Membership{}).Error; err != nil {
		t.Fatalf("remove membership: %v", err)
	}
	title := "edited without changing reference"
	if err := svc.Modify(created.UUID, ModifyInput{Title: &title}); err != nil {
		t.Fatalf("Modify unchanged historical reference: %v", err)
	}
}

func TestAddRejectsNewReferenceWithoutTaskReadScope(t *testing.T) {
	svc, closeFn := newTestService(t, 1000)
	defer closeFn()
	target, err := svc.Add(AddInput{Title: "read-protected target"})
	if err != nil {
		t.Fatal(err)
	}
	token, err := svc.CreateTenantAccessToken(CreateTenantAccessTokenInput{
		Name: "write-only", Scopes: []string{"task:write"}, WorkspaceRef: svc.Runtime().WorkspaceSlug,
	})
	if err != nil {
		t.Fatal(err)
	}
	writeOnly := mustTenantServiceForTest(t, svc, token.RawToken, "task:write", PermissionTaskWrite, 1000)
	description := fmt.Sprintf("[#target](ref://task/%s)", target.UUID)
	_, err = writeOnly.Add(AddInput{Title: "must not reference unreadable task", Description: &description})
	assertRuntimeCode(t, err, "permission_denied")
}

func TestTaskSeriesRejectsInvalidRichDescription(t *testing.T) {
	svc, closeFn := newTestService(t, 1000)
	defer closeFn()
	project, err := svc.AddProject(AddProjectInput{Slug: "richseries", Name: "Series Rich"})
	if err != nil {
		t.Fatal(err)
	}
	description := strings.Repeat("x", maxDescriptionBytes+1)
	_, err = svc.AddTaskSeries(AddTaskSeriesInput{
		Title: "too large", Description: &description, ProjectID: project.ID,
		RecurrenceRule: "daily", FirstDue: 2000,
	})
	assertRuntimeCode(t, err, "description_too_large")
}

func TestTaskSeriesRejectsAttachmentDescriptionReferenceOnCreateAndModify(t *testing.T) {
	svc, closeFn := newTestService(t, 1000)
	defer closeFn()
	project, err := svc.AddProject(AddProjectInput{Slug: "seriesrefs", Name: "Series references"})
	if err != nil {
		t.Fatal(err)
	}
	attachmentDescription := "![图](ref://attachment/00000000-0000-4000-8000-000000000001)"
	_, err = svc.AddTaskSeries(AddTaskSeriesInput{
		Title: "attachment ref", Description: &attachmentDescription, ProjectID: project.ID,
		RecurrenceRule: "daily", FirstDue: 2000,
	})
	assertRuntimeCode(t, err, "description_reference_invalid")

	created, err := svc.AddTaskSeries(AddTaskSeriesInput{
		Title: "normal series", ProjectID: project.ID, RecurrenceRule: "daily", FirstDue: 2000,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.ModifyTaskSeries(created.Series.ID, ModifyTaskSeriesInput{Description: &attachmentDescription})
	assertRuntimeCode(t, err, "description_reference_invalid")
}

func TestAttachmentActivateDrafts(t *testing.T) {
	svc, closeFn := attachmentTestEnv(t)
	defer closeFn()
	tsk, _ := svc.Add(AddInput{Title: "activate-task"})
	payload := pngBytes2x2()
	view, _ := svc.UploadAttachment(context.Background(), "task", tsk.UUID, AttachmentUploadInput{
		Reader:       bytes.NewReader(payload),
		DeclaredSize: int64(len(payload)),
		OriginalName: "draft.png",
		Mode:         "description_draft",
	})
	if err := svc.ActivateDescriptionDrafts(tsk.UUID, []string{view.ID}); err != nil {
		t.Fatalf("ActivateDescriptionDrafts: %v", err)
	}
	list, _ := svc.ListAttachments("task", tsk.UUID, false)
	if len(list) != 1 || list[0].ID != view.ID {
		t.Fatalf("draft not activated: %#v", list)
	}
}

func TestAttachmentUnknownTargetType(t *testing.T) {
	svc, closeFn := attachmentTestEnv(t)
	defer closeFn()
	_, err := svc.ListAttachments("project", "p1", false)
	assertRuntimeCode(t, err, "attachment_target_type_unsupported")
}

func TestAttachmentGetMissing(t *testing.T) {
	svc, closeFn := attachmentTestEnv(t)
	defer closeFn()
	_, err := svc.GetAttachment(uuid.NewString())
	assertRuntimeCode(t, err, "attachment_not_found")
}

func TestAttachmentRename(t *testing.T) {
	svc, closeFn := attachmentTestEnv(t)
	defer closeFn()
	tsk, _ := svc.Add(AddInput{Title: "rename-task"})
	payload := pngBytes2x2()
	view, _ := svc.UploadAttachment(context.Background(), "task", tsk.UUID, AttachmentUploadInput{
		Reader:       bytes.NewReader(payload),
		DeclaredSize: int64(len(payload)),
		OriginalName: "old.png",
		DisplayName:  "old",
		Mode:         "attachment",
	})
	updated, err := svc.RenameAttachment(view.ID, "new name")
	if err != nil {
		t.Fatalf("RenameAttachment: %v", err)
	}
	if updated.DisplayName != "new name" {
		t.Fatalf("display name = %q", updated.DisplayName)
	}
}

func TestAttachmentRemoveActiveWhenNotReferenced(t *testing.T) {
	svc, closeFn := attachmentTestEnv(t)
	defer closeFn()
	tsk, _ := svc.Add(AddInput{Title: "remove-task"})
	payload := pngBytes2x2()
	view, _ := svc.UploadAttachment(context.Background(), "task", tsk.UUID, AttachmentUploadInput{
		Reader:       bytes.NewReader(payload),
		DeclaredSize: int64(len(payload)),
		OriginalName: "remove.png",
		Mode:         "attachment",
	})
	if err := svc.RemoveAttachment(context.Background(), view.ID); err != nil {
		t.Fatalf("RemoveAttachment: %v", err)
	}
	list, _ := svc.ListAttachments("task", tsk.UUID, false)
	if len(list) != 0 {
		t.Fatalf("attachment not removed: %#v", list)
	}
}

func TestAttachmentRemoveActiveWhenReferencedByDescription(t *testing.T) {
	svc, closeFn := attachmentTestEnv(t)
	defer closeFn()
	tsk, _ := svc.Add(AddInput{Title: "ref-task"})
	payload := pngBytes2x2()
	view, _ := svc.UploadAttachment(context.Background(), "task", tsk.UUID, AttachmentUploadInput{
		Reader:       bytes.NewReader(payload),
		DeclaredSize: int64(len(payload)),
		OriginalName: "img.png",
		Mode:         "attachment",
	})
	description := "![](ref://attachment/" + view.ID + ")"
	if err := svc.Modify(tsk.UUID, ModifyInput{Description: &description}); err != nil {
		t.Fatalf("Modify: %v", err)
	}
	err := svc.RemoveAttachment(context.Background(), view.ID)
	assertRuntimeCode(t, err, "attachment_in_use")
}

func TestAttachmentOpenContent(t *testing.T) {
	svc, closeFn := attachmentTestEnv(t)
	defer closeFn()
	tsk, _ := svc.Add(AddInput{Title: "content-task"})
	payload := pngBytes2x2()
	view, _ := svc.UploadAttachment(context.Background(), "task", tsk.UUID, AttachmentUploadInput{
		Reader:       bytes.NewReader(payload),
		DeclaredSize: int64(len(payload)),
		OriginalName: "c.png",
		Mode:         "attachment",
	})
	content, err := svc.OpenAttachmentContent(context.Background(), view.ID)
	if err != nil {
		t.Fatalf("OpenAttachmentContent: %v", err)
	}
	defer content.Reader.Close()
	buf := make([]byte, len(payload))
	n, _ := content.Reader.Read(buf)
	if !bytes.Equal(buf[:n], payload[:n]) {
		t.Fatalf("content mismatch")
	}
	if content.View.ID != view.ID {
		t.Fatalf("view id mismatch")
	}
}

func TestAttachmentRejectsBadMode(t *testing.T) {
	svc, closeFn := attachmentTestEnv(t)
	defer closeFn()
	tsk, _ := svc.Add(AddInput{Title: "bad-mode"})
	_, err := svc.UploadAttachment(context.Background(), "task", tsk.UUID, AttachmentUploadInput{
		Reader:       bytes.NewReader([]byte("x")),
		OriginalName: "x.png",
		Mode:         "invalid",
	})
	assertRuntimeCode(t, err, "attachment_state_invalid")
}

func TestAttachmentRejectsBlockedFileType(t *testing.T) {
	svc, closeFn := attachmentTestEnv(t)
	defer closeFn()
	tsk, _ := svc.Add(AddInput{Title: "bad-type"})
	_, err := svc.UploadAttachment(context.Background(), "task", tsk.UUID, AttachmentUploadInput{
		Reader:       bytes.NewReader([]byte("MZ")),
		OriginalName: "evil.exe",
		Mode:         "attachment",
	})
	assertRuntimeCode(t, err, "attachment_type_not_allowed")
}

func TestAttachmentCleanupRemovesExpiredDraft(t *testing.T) {
	svc, closeFn := attachmentTestEnv(t)
	defer closeFn()
	tsk, _ := svc.Add(AddInput{Title: "cleanup-task"})
	payload := pngBytes2x2()
	view, _ := svc.UploadAttachment(context.Background(), "task", tsk.UUID, AttachmentUploadInput{
		Reader:       bytes.NewReader(payload),
		DeclaredSize: int64(len(payload)),
		OriginalName: "draft.png",
		Mode:         "description_draft",
	})
	// 把时间推到 TTL 之后。
	svc.clock = FixedClock{NowUnix: 1000 + 25*3600}
	result, err := svc.CleanupAttachments(context.Background(), 10)
	if err != nil {
		t.Fatalf("CleanupAttachments: %v", err)
	}
	if result.Scanned == 0 || result.Removed == 0 {
		t.Fatalf("cleanup result = %#v", result)
	}
	_ = view
}

// 避免循环导入 storage 包；这里直接调用 storage.Open。

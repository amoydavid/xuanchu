package app

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"path/filepath"
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

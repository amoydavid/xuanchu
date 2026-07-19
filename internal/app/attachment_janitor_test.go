package app

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"
	"time"

	"git.dajee.net/dajee/xuanchu/internal/attachments"
	"git.dajee.net/dajee/xuanchu/internal/storage"
)

func newJanitorTestEnv(t *testing.T, now int64) (*Service, *AttachmentJanitor, *storage.Store, func()) {
	t.Helper()
	store, err := storage.Open(filepath.Join(t.TempDir(), "xuanchu.db"))
	if err != nil {
		t.Fatal(err)
	}
	cfg := attachments.DefaultConfig(t.TempDir())
	cfg.MaxFileSizeBytes = 256 * 1024
	cfg.DraftTTL = 1 * time.Hour
	rt, err := NewAttachmentRuntime(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	svc, err := NewService(ServiceOptions{
		Store:       store,
		Clock:       FixedClock{NowUnix: now},
		Attachments: rt,
	})
	if err != nil {
		t.Fatal(err)
	}
	janitor := NewAttachmentJanitor(AttachmentJanitorOptions{
		Store:     store,
		Runtime:   rt,
		Clock:     FixedClock{NowUnix: now},
		BatchSize: 10,
		Logger:    nil,
	})
	return svc, janitor, store, func() { _ = store.Close() }
}

func TestAttachmentJanitorRemovesExpiredDraft(t *testing.T) {
	now := int64(1000)
	svc, janitor, _, closeFn := newJanitorTestEnv(t, now)
	defer closeFn()

	tsk, _ := svc.Add(AddInput{Title: "janitor-task"})
	payload := pngBytes2x2()
	view, err := svc.UploadAttachment(context.Background(), "task", tsk.UUID, AttachmentUploadInput{
		Reader:       bytes.NewReader(payload),
		DeclaredSize: int64(len(payload)),
		OriginalName: "draft.png",
		Mode:         "description_draft",
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = view

	// 推进时间到 draft 过期之后。
	janitor.clock = FixedClock{NowUnix: now + 2*3600}
	result, err := janitor.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if result.Scanned == 0 || result.Removed == 0 {
		t.Fatalf("cleanup result = %#v", result)
	}
}

func TestAttachmentJanitorKeepsActive(t *testing.T) {
	now := int64(1000)
	svc, janitor, _, closeFn := newJanitorTestEnv(t, now)
	defer closeFn()

	tsk, _ := svc.Add(AddInput{Title: "keep-active"})
	payload := pngBytes2x2()
	view, _ := svc.UploadAttachment(context.Background(), "task", tsk.UUID, AttachmentUploadInput{
		Reader:       bytes.NewReader(payload),
		DeclaredSize: int64(len(payload)),
		OriginalName: "active.png",
		Mode:         "attachment",
	})

	janitor.clock = FixedClock{NowUnix: now + 100*3600}
	result, _ := janitor.RunOnce(context.Background())
	if result.Removed != 0 {
		t.Fatalf("active attachment should not be removed: %#v", result)
	}
	// 确认附件还在。
	if _, err := svc.GetAttachment(view.ID); err != nil {
		t.Fatalf("GetAttachment: %v", err)
	}
}

func TestAttachmentJanitorRunStopsOnContextCancel(t *testing.T) {
	now := int64(1000)
	_, janitor, _, closeFn := newJanitorTestEnv(t, now)
	defer closeFn()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消，确认 Run 立即返回不阻塞。
	done := make(chan struct{})
	go func() {
		janitor.Run(ctx)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not stop on ctx cancel")
	}
}

// bytes.NewReader 已直接用于测试 reader。

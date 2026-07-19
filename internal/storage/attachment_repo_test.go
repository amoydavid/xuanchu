package storage

import (
	"errors"
	"path/filepath"
	"testing"
)

func newAttachmentTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := Open(filepath.Join(t.TempDir(), "xuanchu.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func mustCreateAttachment(t *testing.T, repo *AttachmentRepository, row Attachment) Attachment {
	t.Helper()
	created, err := repo.Create(row)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	return created
}

func TestAttachmentRepositoryListScopesByWorkspaceAndTarget(t *testing.T) {
	store := newAttachmentTestStore(t)
	repo := NewAttachmentRepository(store.DB())
	mustCreateAttachment(t, repo, Attachment{ID: "a", WorkspaceID: "ws-a", AttachedToType: "task", AttachedToID: "t-1", State: AttachmentStateActive, SizeBytes: 1, CreatedAt: 1, ModifiedAt: 1, StorageBackend: "filesystem", StorageKey: "k1"})
	mustCreateAttachment(t, repo, Attachment{ID: "b", WorkspaceID: "ws-b", AttachedToType: "task", AttachedToID: "t-1", State: AttachmentStateActive, SizeBytes: 1, CreatedAt: 1, ModifiedAt: 1, StorageBackend: "filesystem", StorageKey: "k2"})

	got, err := repo.List(AttachmentListOptions{WorkspaceID: "ws-a", AttachedToType: "task", AttachedToID: "t-1", States: []string{AttachmentStateActive}})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 || got[0].ID != "a" {
		t.Fatalf("got = %#v", got)
	}
}

func TestAttachmentRepositoryListExcludesDeletedByDefault(t *testing.T) {
	store := newAttachmentTestStore(t)
	repo := NewAttachmentRepository(store.DB())
	mustCreateAttachment(t, repo, Attachment{ID: "a", WorkspaceID: "ws", AttachedToType: "task", AttachedToID: "t", State: AttachmentStateActive, SizeBytes: 1, CreatedAt: 1, ModifiedAt: 1, StorageBackend: "filesystem", StorageKey: "k1"})
	mustCreateAttachment(t, repo, Attachment{ID: "b", WorkspaceID: "ws", AttachedToType: "task", AttachedToID: "t", State: AttachmentStateDeleted, SizeBytes: 1, CreatedAt: 1, ModifiedAt: 1, StorageBackend: "filesystem", StorageKey: "k2"})

	got, _ := repo.List(AttachmentListOptions{WorkspaceID: "ws", AttachedToType: "task", AttachedToID: "t"})
	if len(got) != 1 || got[0].ID != "a" {
		t.Fatalf("got = %#v", got)
	}
	got, _ = repo.List(AttachmentListOptions{WorkspaceID: "ws", AttachedToType: "task", AttachedToID: "t", IncludeDeleted: true})
	if len(got) != 2 {
		t.Fatalf("include deleted got = %#v", got)
	}
}

func TestAttachmentRepositoryListDraftCreatorOnly(t *testing.T) {
	store := newAttachmentTestStore(t)
	repo := NewAttachmentRepository(store.DB())
	mustCreateAttachment(t, repo, Attachment{ID: "a", WorkspaceID: "ws", AttachedToType: "task", AttachedToID: "t", State: AttachmentStateDraft, CreatedBy: "alice", SizeBytes: 1, CreatedAt: 1, ModifiedAt: 1, StorageBackend: "filesystem", StorageKey: "k1"})
	mustCreateAttachment(t, repo, Attachment{ID: "b", WorkspaceID: "ws", AttachedToType: "task", AttachedToID: "t", State: AttachmentStateDraft, CreatedBy: "bob", SizeBytes: 1, CreatedAt: 1, ModifiedAt: 1, StorageBackend: "filesystem", StorageKey: "k2"})
	mustCreateAttachment(t, repo, Attachment{ID: "c", WorkspaceID: "ws", AttachedToType: "task", AttachedToID: "t", State: AttachmentStateActive, CreatedBy: "alice", SizeBytes: 1, CreatedAt: 1, ModifiedAt: 1, StorageBackend: "filesystem", StorageKey: "k3"})

	got, _ := repo.List(AttachmentListOptions{WorkspaceID: "ws", AttachedToType: "task", AttachedToID: "t", DraftCreatorOnly: "alice"})
	// alice 应看到自己的 draft + 所有 active（非 draft 由其他人持有也可见）
	ids := map[string]bool{}
	for _, row := range got {
		ids[row.ID] = true
	}
	if !ids["a"] || !ids["c"] {
		t.Fatalf("alice should see own draft + active, got = %#v", got)
	}
	if ids["b"] {
		t.Fatalf("alice must not see bob's draft, got = %#v", got)
	}
}

func TestAttachmentRepositoryUpdateDisplayName(t *testing.T) {
	store := newAttachmentTestStore(t)
	repo := NewAttachmentRepository(store.DB())
	row := mustCreateAttachment(t, repo, Attachment{ID: "a", WorkspaceID: "ws", AttachedToType: "task", AttachedToID: "t", State: AttachmentStateActive, DisplayName: "old", SizeBytes: 1, CreatedAt: 1, ModifiedAt: 1, StorageBackend: "filesystem", StorageKey: "k1"})
	updated, err := repo.UpdateDisplayName("a", "new", 10)
	if err != nil {
		t.Fatalf("UpdateDisplayName: %v", err)
	}
	if updated.DisplayName != "new" || updated.ModifiedAt != 10 {
		t.Fatalf("got = %#v", updated)
	}
	_ = row
	if _, err := repo.UpdateDisplayName("missing", "x", 1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing err = %v", err)
	}
}

func TestAttachmentRepositoryFinalizeWithQuotaChecksCount(t *testing.T) {
	store := newAttachmentTestStore(t)
	repo := NewAttachmentRepository(store.DB())
	// 创建 1 个 uploading row，Finalize 把它转为 active。
	row := mustCreateAttachment(t, repo, Attachment{
		ID: "a", WorkspaceID: "ws", AttachedToType: "task", AttachedToID: "t",
		State: AttachmentStateUploading, SizeBytes: 0, CreatedAt: 1, ModifiedAt: 1,
		StorageBackend: "filesystem", StorageKey: "k1",
	})
	limits := AttachmentQuotaLimits{MaxAttachmentsPerResource: 2, MaxResourceTotalSizeBytes: 100, MaxWorkspaceTotalSizeBytes: 200}
	final := AttachmentFinalize{
		State: AttachmentStateActive, OriginalName: "a.png", DisplayName: "A",
		MediaType: "image/png", Extension: "png", SizeBytes: 10, SHA256: "h",
		InlineCapable: true, StorageBackend: "filesystem", StorageKey: "k1",
		ModifiedAt: 5,
	}
	if err := repo.FinalizeWithQuota("a", final, limits); err != nil {
		t.Fatalf("first finalize: %v", err)
	}
	got, _ := repo.GetByID("a")
	if got.State != AttachmentStateActive || got.SizeBytes != 10 {
		t.Fatalf("got = %#v", got)
	}
	_ = row

	// 创建第二个 attachment 并 finalize 成功（仍在 2 个上限内）。
	row2 := mustCreateAttachment(t, repo, Attachment{
		ID: "b", WorkspaceID: "ws", AttachedToType: "task", AttachedToID: "t",
		State: AttachmentStateUploading, SizeBytes: 0, CreatedAt: 1, ModifiedAt: 1,
		StorageBackend: "filesystem", StorageKey: "k2",
	})
	if err := repo.FinalizeWithQuota("b", AttachmentFinalize{
		State: AttachmentStateActive, SizeBytes: 5, SHA256: "h2",
		StorageBackend: "filesystem", StorageKey: "k2", ModifiedAt: 5,
	}, limits); err != nil {
		t.Fatalf("second finalize: %v", err)
	}
	_ = row2

	// 第三个超过数量上限。
	mustCreateAttachment(t, repo, Attachment{
		ID: "c", WorkspaceID: "ws", AttachedToType: "task", AttachedToID: "t",
		State: AttachmentStateUploading, SizeBytes: 0, CreatedAt: 1, ModifiedAt: 1,
		StorageBackend: "filesystem", StorageKey: "k3",
	})
	err := repo.FinalizeWithQuota("c", AttachmentFinalize{
		State: AttachmentStateActive, SizeBytes: 1, SHA256: "h3",
		StorageBackend: "filesystem", StorageKey: "k3", ModifiedAt: 5,
	}, limits)
	var qee QuotaExceededError
	if !errors.As(err, &qee) || qee.Scope != "resource_count" {
		t.Fatalf("err = %v, want resource_count quota", err)
	}
}

func TestAttachmentRepositoryFinalizeWithQuotaChecksResourceBytes(t *testing.T) {
	store := newAttachmentTestStore(t)
	repo := NewAttachmentRepository(store.DB())
	mustCreateAttachment(t, repo, Attachment{ID: "a", WorkspaceID: "ws", AttachedToType: "task", AttachedToID: "t", State: AttachmentStateUploading, SizeBytes: 0, CreatedAt: 1, ModifiedAt: 1, StorageBackend: "filesystem", StorageKey: "k1"})
	limits := AttachmentQuotaLimits{MaxAttachmentsPerResource: 100, MaxResourceTotalSizeBytes: 50, MaxWorkspaceTotalSizeBytes: 1000}
	err := repo.FinalizeWithQuota("a", AttachmentFinalize{
		State: AttachmentStateActive, SizeBytes: 60, SHA256: "h",
		StorageBackend: "filesystem", StorageKey: "k1", ModifiedAt: 5,
	}, limits)
	var qee QuotaExceededError
	if !errors.As(err, &qee) || qee.Scope != "resource_bytes" {
		t.Fatalf("err = %v, want resource_bytes quota", err)
	}
}

func TestAttachmentRepositoryActivateDraftsOnlyOwnDrafts(t *testing.T) {
	store := newAttachmentTestStore(t)
	repo := NewAttachmentRepository(store.DB())
	mustCreateAttachment(t, repo, Attachment{ID: "a", WorkspaceID: "ws", AttachedToType: "task", AttachedToID: "t", State: AttachmentStateDraft, CreatedBy: "alice", SizeBytes: 1, CreatedAt: 1, ModifiedAt: 1, StorageBackend: "filesystem", StorageKey: "k1"})
	mustCreateAttachment(t, repo, Attachment{ID: "b", WorkspaceID: "ws", AttachedToType: "task", AttachedToID: "t", State: AttachmentStateDraft, CreatedBy: "bob", SizeBytes: 1, CreatedAt: 1, ModifiedAt: 1, StorageBackend: "filesystem", StorageKey: "k2"})

	n, err := repo.ActivateDrafts("t", "alice", []string{"a", "b"}, 10)
	if err != nil {
		t.Fatalf("ActivateDrafts: %v", err)
	}
	if n != 1 {
		t.Fatalf("activated = %d, want 1", n)
	}
	a, _ := repo.GetByID("a")
	if a.State != AttachmentStateActive || !a.EverEmbedded {
		t.Fatalf("a not activated: %#v", a)
	}
	b, _ := repo.GetByID("b")
	if b.State != AttachmentStateDraft {
		t.Fatalf("bob's draft must not be activated: %#v", b)
	}
}

func TestAttachmentRepositoryMarkDeletedAndJanitor(t *testing.T) {
	store := newAttachmentTestStore(t)
	repo := NewAttachmentRepository(store.DB())
	mustCreateAttachment(t, repo, Attachment{ID: "a", WorkspaceID: "ws", AttachedToType: "task", AttachedToID: "t", State: AttachmentStateActive, SizeBytes: 1, CreatedAt: 1, ModifiedAt: 1, StorageBackend: "filesystem", StorageKey: "k1"})
	if err := repo.MarkDeleted("a", 100, 200); err != nil {
		t.Fatalf("MarkDeleted: %v", err)
	}
	got, _ := repo.GetByID("a")
	if got.State != AttachmentStateDeleted || got.PurgeAfter == nil || *got.PurgeAfter != 200 {
		t.Fatalf("got = %#v", got)
	}
	// purge_after <= now 应被列出。
	rows, err := repo.ListJanitorCandidates(250, 86400, 3600, 10)
	if err != nil {
		t.Fatalf("ListJanitorCandidates: %v", err)
	}
	found := false
	for _, r := range rows {
		if r.ID == "a" {
			found = true
		}
	}
	if !found {
		t.Fatalf("deleted row not in janitor candidates: %#v", rows)
	}
}

func TestAttachmentRepositoryDeleteRow(t *testing.T) {
	store := newAttachmentTestStore(t)
	repo := NewAttachmentRepository(store.DB())
	mustCreateAttachment(t, repo, Attachment{ID: "a", WorkspaceID: "ws", AttachedToType: "task", AttachedToID: "t", State: AttachmentStateActive, SizeBytes: 1, CreatedAt: 1, ModifiedAt: 1, StorageBackend: "filesystem", StorageKey: "k1"})
	if err := repo.DeleteRow("a"); err != nil {
		t.Fatalf("DeleteRow: %v", err)
	}
	if _, err := repo.GetByID("a"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("after delete err = %v", err)
	}
	if err := repo.DeleteRow("missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing err = %v", err)
	}
}

func TestAttachmentRepositoryStaleUploadingJanitor(t *testing.T) {
	store := newAttachmentTestStore(t)
	repo := NewAttachmentRepository(store.DB())
	// 1 小时前创建的 uploading 应该被 janitor 候选。
	mustCreateAttachment(t, repo, Attachment{ID: "old", WorkspaceID: "ws", AttachedToType: "task", AttachedToID: "t", State: AttachmentStateUploading, SizeBytes: 0, CreatedAt: 1, ModifiedAt: 1, StorageBackend: "filesystem", StorageKey: "k1"})
	now := int64(5000)
	rows, err := repo.ListJanitorCandidates(now, 86400, 3600, 10)
	if err != nil {
		t.Fatalf("ListJanitorCandidates: %v", err)
	}
	found := false
	for _, r := range rows {
		if r.ID == "old" {
			found = true
		}
	}
	if !found {
		t.Fatalf("stale uploading not in candidates")
	}
}

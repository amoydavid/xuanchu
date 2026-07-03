package storage

import (
	"path/filepath"
	"testing"
)

func TestDirectorySyncJobMigrated(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "x.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	job := DirectorySyncJob{ID: "j1", WorkspaceID: "ws1", Status: "pending", CreatedAt: 1}
	if err := store.db.Create(&job).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	var got DirectorySyncJob
	if err := store.db.First(&got, "id=?", "j1").Error; err != nil {
		t.Fatalf("query: %v", err)
	}
}

func TestSyncJobCreateRejectsDuplicate(t *testing.T) {
	store, _ := Open(filepath.Join(t.TempDir(), "x.db"))
	t.Cleanup(func() { _ = store.Close() })
	repo := NewDirectorySyncJobRepository(store.db)

	if _, err := repo.Create("ws1", 100); err != nil {
		t.Fatalf("first create: %v", err)
	}
	if _, err := repo.Create("ws1", 101); err != ErrSyncInProgress {
		t.Fatalf("second create err = %v, want ErrSyncInProgress", err)
	}
}

func TestSyncJobClaimAndComplete(t *testing.T) {
	store, _ := Open(filepath.Join(t.TempDir(), "x.db"))
	t.Cleanup(func() { _ = store.Close() })
	repo := NewDirectorySyncJobRepository(store.db)

	job, _ := repo.Create("ws1", 100)
	claimed, err := repo.ClaimNextPending(200, 300)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if claimed.ID != job.ID {
		t.Fatalf("claimed %s, want %s", claimed.ID, job.ID)
	}
	if err := repo.MarkSucceeded(job.ID, 300, `{"added":1}`); err != nil {
		t.Fatalf("succeeded: %v", err)
	}
	// 完成后可再次创建
	if _, err := repo.Create("ws1", 301); err != nil {
		t.Fatalf("create after done: %v", err)
	}
}

func TestSyncJobReclaimExpired(t *testing.T) {
	store, _ := Open(filepath.Join(t.TempDir(), "x.db"))
	t.Cleanup(func() { _ = store.Close() })
	repo := NewDirectorySyncJobRepository(store.db)

	job, _ := repo.Create("ws1", 100)
	// 手动把 claim 设为已过期（claimed_at 久远，claim_expires_at 已过）
	store.db.Model(&DirectorySyncJob{}).Where("id=?", job.ID).
		Updates(map[string]any{"status": "running", "claimed_at": 100, "claim_expires_at": 150})
	claimed, err := repo.ClaimNextPending(500, 300)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if claimed.ID != job.ID {
		t.Fatalf("did not reclaim expired job")
	}
}

func TestSyncJobMarkFailed(t *testing.T) {
	store, _ := Open(filepath.Join(t.TempDir(), "x.db"))
	t.Cleanup(func() { _ = store.Close() })
	repo := NewDirectorySyncJobRepository(store.db)

	job, _ := repo.Create("ws1", 100)
	_, _ = repo.ClaimNextPending(200, 300)
	if err := repo.MarkFailed(job.ID, 300, "boom"); err != nil {
		t.Fatalf("failed: %v", err)
	}
	var got DirectorySyncJob
	store.db.First(&got, "id=?", job.ID)
	if got.Status != "failed" || got.ErrorMessage != "boom" {
		t.Fatalf("got %+v", got)
	}
}

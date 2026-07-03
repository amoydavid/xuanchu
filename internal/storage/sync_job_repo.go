package storage

import (
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

var ErrSyncInProgress = errors.New("sync_in_progress")

type DirectorySyncJobRepository struct {
	db *gorm.DB
}

func NewDirectorySyncJobRepository(db *gorm.DB) *DirectorySyncJobRepository {
	return &DirectorySyncJobRepository{db: db}
}

// Create 插入 pending job；若该 workspace 已有 pending/running job 则返回 ErrSyncInProgress。
// Count + Insert 在同一事务内完成，避免并发 TOCTOU 导致重复创建。
func (r *DirectorySyncJobRepository) Create(workspaceID string, now int64) (DirectorySyncJob, error) {
	job := DirectorySyncJob{
		ID:          "syncjob_" + uuid.NewString(),
		WorkspaceID: workspaceID,
		Status:      "pending",
		CreatedAt:   now,
	}
	err := r.db.Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&DirectorySyncJob{}).
			Where("workspace_id = ? AND status IN ?", workspaceID, []string{"pending", "running"}).
			Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return ErrSyncInProgress
		}
		return tx.Create(&job).Error
	})
	if err != nil {
		return DirectorySyncJob{}, err
	}
	return job, nil
}

// ClaimNextPending 认领最早的 pending job（或 claim 已过期的 running job），用 compare-and-swap 原子置 running。
func (r *DirectorySyncJobRepository) ClaimNextPending(now, claimTTL int64) (DirectorySyncJob, error) {
	var job DirectorySyncJob
	err := r.db.Transaction(func(tx *gorm.DB) error {
		// 优先 pending
		err := tx.Where("status = ?", "pending").Order("created_at ASC").First(&job).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// 回收过期 running（claim_expires_at < now）
			err = tx.Where("status = ? AND claim_expires_at < ?", "running", now).
				Order("created_at ASC").First(&job).Error
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		condition := tx.Model(&DirectorySyncJob{}).Where("id = ?", job.ID)
		if job.Status == "pending" {
			condition = condition.Where("status = ?", "pending")
		} else {
			condition = condition.Where("status = ? AND claim_expires_at < ?", "running", now)
		}
		res := condition.
			Updates(map[string]any{
				"status":           "running",
				"claimed_at":       now,
				"claim_expires_at": now + claimTTL,
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return ErrNotFound
		}
		return nil
	})
	if err != nil {
		return DirectorySyncJob{}, err
	}
	job.Status = "running"
	job.ClaimedAt = &now
	exp := now + claimTTL
	job.ClaimExpiresAt = &exp
	return job, nil
}

func (r *DirectorySyncJobRepository) MarkSucceeded(id string, finishedAt int64, statsJSON string) error {
	return r.db.Model(&DirectorySyncJob{}).Where("id = ?", id).
		Updates(map[string]any{
			"status":      "succeeded",
			"stats_json":  statsJSON,
			"finished_at": finishedAt,
		}).Error
}

func (r *DirectorySyncJobRepository) MarkFailed(id string, finishedAt int64, msg string) error {
	return r.db.Model(&DirectorySyncJob{}).Where("id = ?", id).
		Updates(map[string]any{
			"status":        "failed",
			"error_message": msg,
			"finished_at":   finishedAt,
		}).Error
}

func (r *DirectorySyncJobRepository) Get(id string) (DirectorySyncJob, error) {
	var job DirectorySyncJob
	err := r.db.First(&job, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return DirectorySyncJob{}, ErrNotFound
	}
	return job, err
}

func (r *DirectorySyncJobRepository) LatestForWorkspace(workspaceID string) (DirectorySyncJob, error) {
	var job DirectorySyncJob
	err := r.db.Where("workspace_id = ?", workspaceID).Order("created_at DESC").First(&job).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return DirectorySyncJob{}, ErrNotFound
	}
	return job, err
}

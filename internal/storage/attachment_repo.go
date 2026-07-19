package storage

import (
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// AttachmentListOptions 描述附件列表查询条件。
type AttachmentListOptions struct {
	WorkspaceID    string
	AttachedToType string
	AttachedToID   string
	// States 过滤状态集合；空表示不限制。
	States []string
	// DraftCreatorOnly 仅返回 CreatedBy 匹配 creatorID 的 draft 行；
	// 用于编辑器恢复/清理自己的 draft，不能查看其他 actor 的 draft。
	DraftCreatorOnly string
	// IncludeDeleted 为 true 时返回 deleted；默认排除 deleted。
	IncludeDeleted bool
	// OrderByCreatedDesc 为 true 时按 created_at DESC，否则 ASC。
	OrderByCreatedDesc bool
}

// AttachmentQuotaLimits 描述 FinalizeWithQuota 的配额上限。
type AttachmentQuotaLimits struct {
	MaxResourceTotalSizeBytes int64
	MaxWorkspaceTotalSizeBytes int64
	MaxAttachmentsPerResource int
}

// AttachmentFinalize 是最终提交时的字段更新。
type AttachmentFinalize struct {
	State         string
	OriginalName  string
	DisplayName   string
	MediaType     string
	Extension     string
	SizeBytes     int64
	SHA256        string
	InlineCapable bool
	StorageBackend string
	StorageKey    string
	SourceHost    string
	SourceURLHash string
	SourceType    string
	ModifiedAt    int64
}

// AttachmentRepository 封装附件元数据的持久化。
type AttachmentRepository struct {
	db *gorm.DB
}

// NewAttachmentRepository 构造附件仓储。
func NewAttachmentRepository(db *gorm.DB) *AttachmentRepository {
	return &AttachmentRepository{db: db}
}

// Create 写入一条 attachment row。
func (r *AttachmentRepository) Create(row Attachment) (Attachment, error) {
	if err := r.db.Create(&row).Error; err != nil {
		return Attachment{}, err
	}
	return row, nil
}

// GetByID 按 ID 读取单条。
func (r *AttachmentRepository) GetByID(id string) (Attachment, error) {
	var row Attachment
	err := r.db.Where("id = ?", id).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Attachment{}, ErrNotFound
	}
	if err != nil {
		return Attachment{}, err
	}
	return row, nil
}

// List 按 options 过滤。
func (r *AttachmentRepository) List(opts AttachmentListOptions) ([]Attachment, error) {
	q := r.db.Model(&Attachment{})
	if opts.WorkspaceID != "" {
		q = q.Where("workspace_id = ?", opts.WorkspaceID)
	}
	if opts.AttachedToType != "" {
		q = q.Where("attached_to_type = ?", opts.AttachedToType)
	}
	if opts.AttachedToID != "" {
		q = q.Where("attached_to_id = ?", opts.AttachedToID)
	}
	if len(opts.States) > 0 {
		q = q.Where("state IN ?", opts.States)
	}
	if !opts.IncludeDeleted {
		q = q.Where("state <> ?", AttachmentStateDeleted)
	}
	if opts.DraftCreatorOnly != "" {
		q = q.Where("(state <> ? OR created_by = ?)", AttachmentStateDraft, opts.DraftCreatorOnly)
	}
	order := "created_at ASC"
	if opts.OrderByCreatedDesc {
		order = "created_at DESC"
	}
	q = q.Order(order)
	var rows []Attachment
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// UpdateDisplayName 仅修改展示名。
func (r *AttachmentRepository) UpdateDisplayName(id, displayName string, modifiedAt int64) (Attachment, error) {
	result := r.db.Model(&Attachment{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"display_name": displayName,
			"modified_at":  modifiedAt,
		})
	if result.Error != nil {
		return Attachment{}, result.Error
	}
	if result.RowsAffected == 0 {
		return Attachment{}, ErrNotFound
	}
	return r.GetByID(id)
}

// FinalizeWithQuota 在同一 transaction 内更新最终 metadata/state，
// 并按 attached resource/workspace 配额做并发安全的最终校验。
//
// resource/workspace 字节统计包含 uploading/draft/active/deleted（retention 内仍占配额）；
// 数量上限只算 uploading/draft/active。
// PostgreSQL 使用 SELECT ... FOR UPDATE 串行化，SQLite 靠 transaction 串行化。
func (r *AttachmentRepository) FinalizeWithQuota(id string, final AttachmentFinalize, limits AttachmentQuotaLimits) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		var row Attachment
		if err := tx.Where("id = ?", id).First(&row).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			return err
		}

		// 数量上限：uploading/draft/active。
		var countActive int64
		countQuery := tx.Model(&Attachment{}).
			Where("workspace_id = ? AND attached_to_type = ? AND attached_to_id = ? AND state IN ?",
				row.WorkspaceID, row.AttachedToType, row.AttachedToID,
				[]string{AttachmentStateUploading, AttachmentStateDraft, AttachmentStateActive})
		if tx.Dialector.Name() == "postgres" {
			countQuery = countQuery.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		if err := countQuery.Count(&countActive).Error; err != nil {
			return err
		}
		if limits.MaxAttachmentsPerResource > 0 && countActive > int64(limits.MaxAttachmentsPerResource) {
			return QuotaExceededError{Scope: "resource_count"}
		}

		// 字节上限：当前 finalize 后的本次 size 替换 row.SizeBytes。
		nextSize := final.SizeBytes
		var resourceBytes int64
		resourceQuery := tx.Model(&Attachment{}).
			Select("COALESCE(SUM(size_bytes), 0)").
			Where("workspace_id = ? AND attached_to_type = ? AND attached_to_id = ?",
				row.WorkspaceID, row.AttachedToType, row.AttachedToID)
		if tx.Dialector.Name() == "postgres" {
			resourceQuery = resourceQuery.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		if err := resourceQuery.Scan(&resourceBytes).Error; err != nil {
			return err
		}
		resourceBytes = resourceBytes - row.SizeBytes + nextSize
		if limits.MaxResourceTotalSizeBytes > 0 && resourceBytes > limits.MaxResourceTotalSizeBytes {
			return QuotaExceededError{Scope: "resource_bytes"}
		}

		var workspaceBytes int64
		workspaceQuery := tx.Model(&Attachment{}).
			Select("COALESCE(SUM(size_bytes), 0)").
			Where("workspace_id = ?", row.WorkspaceID)
		if tx.Dialector.Name() == "postgres" {
			workspaceQuery = workspaceQuery.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		if err := workspaceQuery.Scan(&workspaceBytes).Error; err != nil {
			return err
		}
		workspaceBytes = workspaceBytes - row.SizeBytes + nextSize
		if limits.MaxWorkspaceTotalSizeBytes > 0 && workspaceBytes > limits.MaxWorkspaceTotalSizeBytes {
			return QuotaExceededError{Scope: "workspace_bytes"}
		}

		updates := map[string]any{
			"state":          final.State,
			"original_name":  final.OriginalName,
			"display_name":   final.DisplayName,
			"media_type":     final.MediaType,
			"extension":      final.Extension,
			"size_bytes":     final.SizeBytes,
			"sha256":         final.SHA256,
			"inline_capable": final.InlineCapable,
			"storage_backend": final.StorageBackend,
			"storage_key":    final.StorageKey,
			"modified_at":    final.ModifiedAt,
		}
		if final.SourceType != "" {
			updates["source_type"] = final.SourceType
		}
		if final.SourceHost != "" {
			updates["source_host"] = final.SourceHost
		}
		if final.SourceURLHash != "" {
			updates["source_url_hash"] = final.SourceURLHash
		}
		if err := tx.Model(&Attachment{}).Where("id = ?", id).Updates(updates).Error; err != nil {
			return err
		}
		return nil
	})
}

// ActivateDrafts 把给定 ID 集合中归属当前 task 且由 creator 创建的 draft 转为 active。
//
// 只有 creatorID 匹配的 draft 才会激活；归属不匹配的 ID 静默跳过
// (App 层应在调用前先校验归属)。EverEmbedded 永不回退。
func (r *AttachmentRepository) ActivateDrafts(taskID, creatorID string, ids []string, now int64) (int, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	result := r.db.Model(&Attachment{}).
		Where("id IN ? AND attached_to_type = ? AND attached_to_id = ? AND state = ? AND created_by = ?",
			ids, AttachmentAttachedToTask, taskID, AttachmentStateDraft, creatorID).
		Updates(map[string]any{
			"state":         AttachmentStateActive,
			"ever_embedded": true,
			"modified_at":   now,
		})
	if result.Error != nil {
		return 0, result.Error
	}
	return int(result.RowsAffected), nil
}

// MarkDeleted 把附件软删除并设置 PurgeAfter。
func (r *AttachmentRepository) MarkDeleted(id string, now, purgeAfter int64) error {
	result := r.db.Model(&Attachment{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"state":       AttachmentStateDeleted,
			"deleted_at":  now,
			"purge_after": purgeAfter,
			"modified_at": now,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteRow 物理删除 metadata row。
func (r *AttachmentRepository) DeleteRow(id string) error {
	result := r.db.Where("id = ?", id).Delete(&Attachment{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// ListJanitorCandidates 返回需要清理的候选：stale uploading、过期 draft、到期 deleted。
//
// stale uploading 阈值固定为 1 小时；draft 用 ttl；deleted 用 purge_after <= now。
func (r *AttachmentRepository) ListJanitorCandidates(now int64, draftTTLSeconds, staleUploadingSeconds int64, limit int) ([]Attachment, error) {
	staleUploadingCutoff := now - staleUploadingSeconds
	draftCutoff := now - draftTTLSeconds
	q := r.db.Model(&Attachment{}).Where(
		"(state = ? AND created_at < ?) OR (state = ? AND modified_at < ?) OR (state = ? AND purge_after IS NOT NULL AND purge_after <= ?)",
		AttachmentStateUploading, staleUploadingCutoff,
		AttachmentStateDraft, draftCutoff,
		AttachmentStateDeleted, now,
	)
	if limit > 0 {
		q = q.Limit(limit)
	}
	var rows []Attachment
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// QuotaExceededError 表示附件配额越限。
type QuotaExceededError struct {
	Scope string
}

func (e QuotaExceededError) Error() string { return "attachment quota exceeded: " + e.Scope }

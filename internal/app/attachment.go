package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"

	"git.dajee.net/dajee/xuanchu/internal/attachments"
	"git.dajee.net/dajee/xuanchu/internal/blobstore"
	"git.dajee.net/dajee/xuanchu/internal/safefetch"
	"git.dajee.net/dajee/xuanchu/internal/storage"
	domain "git.dajee.net/dajee/xuanchu/internal/task"
)

// AttachmentTargetView 是对外暴露的附件归属。
type AttachmentTargetView struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

// AttachmentTarget 描述附件归属的业务实体。
type AttachmentTarget struct {
	Type        string
	ID          string
	WorkspaceID string
}

// AttachmentView 是附件的对外视图。
//
// 永不包含 storage backend/key/source URL 等敏感字段。
type AttachmentView struct {
	ID            string               `json:"id"`
	AttachedTo    AttachmentTargetView `json:"attached_to"`
	State         string               `json:"state"`
	OriginalName  string               `json:"original_name"`
	DisplayName   string               `json:"display_name"`
	MediaType     string               `json:"media_type"`
	Extension     string               `json:"extension"`
	SizeBytes     int64                `json:"size_bytes"`
	SHA256        string               `json:"sha256"`
	InlineCapable bool                 `json:"inline_capable"`
	SourceType    string               `json:"source_type"`
	ContentURL    string               `json:"content_url"`
	CreatedBy     domain.JSONActorInfo `json:"created_by"`
	CreatedAt     int64                `json:"created_at"`
	ModifiedAt    int64                `json:"modified_at"`
}

// AttachmentUploadInput 描述上传附件的输入。
type AttachmentUploadInput struct {
	Reader       io.Reader
	DeclaredSize int64
	OriginalName string
	DisplayName  string
	Mode         string // attachment|description_draft
}

// AttachmentImportURLInput 描述远程图片转存输入。
type AttachmentImportURLInput struct {
	SourceURL   string
	DisplayName string
	Mode        string
}

// AttachmentContent 描述 OpenAttachmentContent 返回的内容流。
type AttachmentContent struct {
	View   AttachmentView
	Reader io.ReadCloser
	Blob   blobstore.BlobInfo
}

// AttachmentCleanupResult 描述清理运行的统计。
type AttachmentCleanupResult struct {
	Scanned int
	Removed int
	Retried int
}

const (
	attachmentModeAttachment       = "attachment"
	attachmentModeDescriptionDraft = "description_draft"
	staleUploadingSeconds          = 3600
)

// UploadAttachment 上传一个附件到指定 task。
func (s *Service) UploadAttachment(ctx context.Context, targetType, targetRef string, in AttachmentUploadInput) (AttachmentView, error) {
	if s.attachmentRuntime == nil {
		return AttachmentView{}, RuntimeError{Code: "attachment_storage_unavailable", Message: "attachments not configured"}
	}
	if in.Mode != attachmentModeAttachment && in.Mode != attachmentModeDescriptionDraft {
		return AttachmentView{}, RuntimeError{Code: "attachment_state_invalid", Message: "mode must be attachment or description_draft"}
	}
	target, err := s.resolveAttachmentTarget(targetType, targetRef, true)
	if err != nil {
		return AttachmentView{}, err
	}
	store, err := s.attachmentRuntime.StoreFor(s.attachmentRuntime.Config.Backend)
	if err != nil {
		return AttachmentView{}, err
	}
	inspected, cleanup, err := attachments.InspectToTemp(ctx, in.Reader, in.OriginalName, in.DeclaredSize, s.attachmentRuntime.Config.MaxFileSizeBytes)
	if err != nil {
		return AttachmentView{}, mapInspectError(err)
	}
	defer cleanup()

	attachmentID := uuid.NewString()
	storageKey := attachmentStorageKey(target.WorkspaceID, attachmentID)
	now := s.clock.Unix()

	row := buildAttachmentRow(attachmentID, target, inspected, in, s.attachmentRuntime.Config.Backend, storageKey, s.runtime, now)
	if _, err := s.attachmentRepo.Create(row); err != nil {
		return AttachmentView{}, err
	}

	src, err := os.Open(inspected.Path)
	if err != nil {
		_ = s.attachmentRepo.DeleteRow(attachmentID)
		return AttachmentView{}, RuntimeError{Code: "attachment_storage_unavailable", Message: err.Error()}
	}
	putErr := store.Put(ctx, storageKey, src, inspected.SizeBytes, inspected.MediaType)
	src.Close()
	if putErr != nil {
		s.bestEffortDeleteBlob(ctx, store, storageKey)
		_ = s.attachmentRepo.DeleteRow(attachmentID)
		return AttachmentView{}, RuntimeError{Code: "attachment_storage_unavailable", Message: putErr.Error()}
	}

	state := storage.AttachmentStateActive
	if in.Mode == attachmentModeDescriptionDraft {
		state = storage.AttachmentStateDraft
	}
	displayName := strings.TrimSpace(in.DisplayName)
	if displayName == "" {
		displayName = inspected.OriginalNameFromExt()
	}
	final := storage.AttachmentFinalize{
		State:          state,
		OriginalName:   inspected.OriginalName,
		DisplayName:    displayName,
		MediaType:      inspected.MediaType,
		Extension:      inspected.Extension,
		SizeBytes:      inspected.SizeBytes,
		SHA256:         inspected.SHA256,
		InlineCapable:  inspected.InlineCapable,
		StorageBackend: s.attachmentRuntime.Config.Backend,
		StorageKey:     storageKey,
		SourceType:     "upload",
		ModifiedAt:     now,
	}
	if err := s.attachmentRepo.FinalizeWithQuota(attachmentID, final, s.quotaLimits()); err != nil {
		s.bestEffortDeleteBlob(ctx, store, storageKey)
		_ = s.attachmentRepo.DeleteRow(attachmentID)
		return AttachmentView{}, mapQuotaError(err)
	}

	row, _ = s.attachmentRepo.GetByID(attachmentID)
	s.appendAuditEntry(AuditEntry{
		Action:      "attachment.add",
		WorkspaceID: strPtr(target.WorkspaceID),
		TargetType:  "attachment",
		TargetID:    attachmentID,
		Payload:     attachmentAuditPayload(row, target),
	})
	return s.attachmentView(row)
}

// ImportAttachmentURL 远程图片转存。
func (s *Service) ImportAttachmentURL(ctx context.Context, targetType, targetRef string, in AttachmentImportURLInput) (AttachmentView, error) {
	if s.attachmentRuntime == nil {
		return AttachmentView{}, RuntimeError{Code: "attachment_storage_unavailable", Message: "attachments not configured"}
	}
	if in.Mode != attachmentModeDescriptionDraft {
		return AttachmentView{}, RuntimeError{Code: "attachment_state_invalid", Message: "remote import only supports description_draft"}
	}
	fetcher, ok := s.attachmentRuntime.Fetcher.(*safefetch.Fetcher)
	if !ok || fetcher == nil {
		return AttachmentView{}, RuntimeError{Code: "attachment_remote_fetch_disabled", Message: "remote fetch is disabled"}
	}
	if _, err := s.resolveAttachmentTarget(targetType, targetRef, true); err != nil {
		return AttachmentView{}, err
	}
	result, err := fetcher.Fetch(ctx, in.SourceURL)
	if err != nil {
		return AttachmentView{}, mapFetchError(err)
	}
	defer result.Body.Close()

	displayName := strings.TrimSpace(in.DisplayName)
	if displayName == "" {
		displayName = result.FileName
	}
	if displayName == "" {
		displayName = "remote-image"
	}
	view, err := s.UploadAttachment(ctx, targetType, targetRef, AttachmentUploadInput{
		Reader:       result.Body,
		DeclaredSize: result.ContentLength,
		OriginalName: displayName,
		DisplayName:  displayName,
		Mode:         in.Mode,
	})
	if err != nil {
		return AttachmentView{}, err
	}
	if view.ID != "" {
		host := strings.ToLower(result.SourceHost)
		hash := safefetch.SourceURLHash(in.SourceURL)
		_ = s.attachmentRepo.UpdateRemoteSource(view.ID, host, hash, "remote_url", s.clock.Unix())
	}
	return view, nil
}

// ListAttachments 列出 task 的附件。
func (s *Service) ListAttachments(targetType, targetRef string, includeDrafts bool) ([]AttachmentView, error) {
	if s.attachmentRuntime == nil {
		return nil, RuntimeError{Code: "attachment_storage_unavailable", Message: "attachments not configured"}
	}
	target, err := s.resolveAttachmentTarget(targetType, targetRef, false)
	if err != nil {
		return nil, err
	}
	opts := storage.AttachmentListOptions{
		WorkspaceID:    target.WorkspaceID,
		AttachedToType: target.Type,
		AttachedToID:   target.ID,
	}
	if !includeDrafts {
		opts.States = []string{storage.AttachmentStateActive}
	} else {
		opts.DraftCreatorOnly = s.actorCreatorID()
	}
	rows, err := s.attachmentRepo.List(opts)
	if err != nil {
		return nil, err
	}
	out := make([]AttachmentView, 0, len(rows))
	for _, row := range rows {
		view, err := s.attachmentView(row)
		if err != nil {
			return nil, err
		}
		out = append(out, view)
	}
	return out, nil
}

// GetAttachment 返回单个附件 metadata。
func (s *Service) GetAttachment(id string) (AttachmentView, error) {
	if s.attachmentRuntime == nil {
		return AttachmentView{}, RuntimeError{Code: "attachment_storage_unavailable", Message: "attachments not configured"}
	}
	row, err := s.attachmentRepo.GetByID(id)
	if err != nil {
		return AttachmentView{}, mapRepoNotFound(err)
	}
	if _, err := s.resolveAttachmentTarget(row.AttachedToType, row.AttachedToID, false); err != nil {
		return AttachmentView{}, RuntimeError{Code: "attachment_not_found", Message: "attachment not found"}
	}
	return s.attachmentView(row)
}

// OpenAttachmentContent 流式打开附件内容。
func (s *Service) OpenAttachmentContent(ctx context.Context, id string) (AttachmentContent, error) {
	if s.attachmentRuntime == nil {
		return AttachmentContent{}, RuntimeError{Code: "attachment_storage_unavailable", Message: "attachments not configured"}
	}
	row, err := s.attachmentRepo.GetByID(id)
	if err != nil {
		return AttachmentContent{}, mapRepoNotFound(err)
	}
	if _, err := s.resolveAttachmentTarget(row.AttachedToType, row.AttachedToID, false); err != nil {
		return AttachmentContent{}, RuntimeError{Code: "attachment_not_found", Message: "attachment not found"}
	}
	if row.State == storage.AttachmentStateDeleted && row.PurgeAfter != nil && *row.PurgeAfter <= s.clock.Unix() {
		return AttachmentContent{}, RuntimeError{Code: "attachment_content_gone", Message: "content has been purged"}
	}
	store, err := s.attachmentRuntime.StoreFor(row.StorageBackend)
	if err != nil {
		return AttachmentContent{}, err
	}
	reader, info, err := store.Open(ctx, row.StorageKey)
	if err != nil {
		return AttachmentContent{}, RuntimeError{Code: "attachment_storage_unavailable", Message: "open failed"}
	}
	view, err := s.attachmentView(row)
	if err != nil {
		reader.Close()
		return AttachmentContent{}, err
	}
	return AttachmentContent{View: view, Reader: reader, Blob: info}, nil
}

// RenameAttachment 修改附件展示名。
func (s *Service) RenameAttachment(id, displayName string) (AttachmentView, error) {
	if s.attachmentRuntime == nil {
		return AttachmentView{}, RuntimeError{Code: "attachment_storage_unavailable", Message: "attachments not configured"}
	}
	row, err := s.attachmentRepo.GetByID(id)
	if err != nil {
		return AttachmentView{}, mapRepoNotFound(err)
	}
	target, err := s.resolveAttachmentTarget(row.AttachedToType, row.AttachedToID, true)
	if err != nil {
		return AttachmentView{}, RuntimeError{Code: "attachment_not_found", Message: "attachment not found"}
	}
	displayName = strings.TrimSpace(displayName)
	if displayName == "" {
		return AttachmentView{}, RuntimeError{Code: "attachment_state_invalid", Message: "display name required"}
	}
	updated, err := s.attachmentRepo.UpdateDisplayName(id, displayName, s.clock.Unix())
	if err != nil {
		return AttachmentView{}, err
	}
	s.appendAuditEntry(AuditEntry{
		Action:      "attachment.rename",
		WorkspaceID: strPtr(row.WorkspaceID),
		TargetType:  "attachment",
		TargetID:    id,
		Payload:     attachmentAuditPayload(updated, target),
	})
	return s.attachmentView(updated)
}

// RemoveAttachment 删除附件。
func (s *Service) RemoveAttachment(ctx context.Context, id string) error {
	if s.attachmentRuntime == nil {
		return RuntimeError{Code: "attachment_storage_unavailable", Message: "attachments not configured"}
	}
	row, err := s.attachmentRepo.GetByID(id)
	if err != nil {
		return mapRepoNotFound(err)
	}
	target, err := s.resolveAttachmentTarget(row.AttachedToType, row.AttachedToID, true)
	if err != nil {
		return RuntimeError{Code: "attachment_not_found", Message: "attachment not found"}
	}
	switch row.State {
	case storage.AttachmentStateActive:
		if used, _ := s.attachmentReferencedByDescription(row); used {
			return RuntimeError{Code: "attachment_in_use", Message: "attachment is referenced by description"}
		}
		purgeAfter := s.clock.Unix() + int64(s.attachmentRuntime.Config.DeletedRetention/time.Second)
		if err := s.attachmentRepo.MarkDeleted(id, s.clock.Unix(), purgeAfter); err != nil {
			return err
		}
	case storage.AttachmentStateDraft:
		if store, err := s.attachmentRuntime.StoreFor(row.StorageBackend); err == nil {
			s.bestEffortDeleteBlob(ctx, store, row.StorageKey)
		}
		if err := s.attachmentRepo.DeleteRow(id); err != nil {
			return err
		}
	case storage.AttachmentStateDeleted:
		return nil
	default:
		return RuntimeError{Code: "attachment_state_invalid", Message: "cannot remove attachment in state " + row.State}
	}
	s.appendAuditEntry(AuditEntry{
		Action:      "attachment.remove",
		WorkspaceID: strPtr(target.WorkspaceID),
		TargetType:  "attachment",
		TargetID:    id,
		Payload:     attachmentAuditPayload(row, target),
	})
	return nil
}

// ActivateDescriptionDrafts 把 task description 引用到的 draft attachment 转为 active。
func (s *Service) ActivateDescriptionDrafts(taskID string, ids []string) error {
	if s.attachmentRuntime == nil {
		return RuntimeError{Code: "attachment_storage_unavailable", Message: "attachments not configured"}
	}
	creator := s.actorCreatorID()
	if creator == "" {
		return RuntimeError{Code: "attachment_draft_creator_mismatch", Message: "draft actor required"}
	}
	if _, err := s.attachmentRepo.ActivateDrafts(taskID, creator, ids, s.clock.Unix()); err != nil {
		return err
	}
	return nil
}

// CleanupAttachments 执行一轮小批量清理。
func (s *Service) CleanupAttachments(ctx context.Context, limit int) (AttachmentCleanupResult, error) {
	result := AttachmentCleanupResult{}
	if s.attachmentRuntime == nil {
		return result, RuntimeError{Code: "attachment_storage_unavailable", Message: "attachments not configured"}
	}
	if limit <= 0 {
		limit = 100
	}
	draftTTLSeconds := int64(s.attachmentRuntime.Config.DraftTTL / time.Second)
	rows, err := s.attachmentRepo.ListJanitorCandidates(s.clock.Unix(), draftTTLSeconds, staleUploadingSeconds, limit)
	if err != nil {
		return result, err
	}
	result.Scanned = len(rows)
	for _, row := range rows {
		store, storeErr := s.attachmentRuntime.StoreFor(row.StorageBackend)
		if storeErr != nil {
			continue
		}
		if err := store.Delete(ctx, row.StorageKey); err != nil {
			result.Retried++
			continue
		}
		if err := s.attachmentRepo.DeleteRow(row.ID); err != nil {
			result.Retried++
			continue
		}
		result.Removed++
	}
	return result, nil
}

// resolveAttachmentTarget 解析 task target，执行 workspace/scope/closed-state 检查。
func (s *Service) resolveAttachmentTarget(targetType, targetRef string, write bool) (AttachmentTarget, error) {
	if targetType != storage.AttachmentAttachedToTask {
		return AttachmentTarget{}, RuntimeError{Code: "attachment_target_type_unsupported", Message: "only task target is supported"}
	}
	if write {
		if err := s.Require(PermissionTaskWrite); err != nil {
			return AttachmentTarget{}, err
		}
	} else {
		if err := s.Require(PermissionTaskRead); err != nil {
			return AttachmentTarget{}, err
		}
	}
	tsk, err := s.resolveTargetForRead(targetRef)
	if err != nil {
		return AttachmentTarget{}, err
	}
	if write {
		if _, err := s.resolveTargetForWrite(targetRef); err != nil {
			return AttachmentTarget{}, err
		}
	}
	if tsk.WorkspaceID != s.workspaceID {
		return AttachmentTarget{}, RuntimeError{Code: "attachment_not_found", Message: "attachment not found"}
	}
	return AttachmentTarget{Type: storage.AttachmentAttachedToTask, ID: tsk.UUID, WorkspaceID: tsk.WorkspaceID}, nil
}

// attachmentView 把 storage.Attachment 转换为对外视图。
func (s *Service) attachmentView(row storage.Attachment) (AttachmentView, error) {
	actor, err := s.actorInfoFromColumns(attachmentActorColumns(row), row.CreatedBy)
	if err != nil {
		// 解析失败时降级为 fallback，避免单个附件拖垮整个列表。
		actor = actorInfoFromColumns(attachmentActorColumns(row), row.CreatedBy, nil)
	}
	return AttachmentView{
		ID:            row.ID,
		AttachedTo:    AttachmentTargetView{Type: row.AttachedToType, ID: row.AttachedToID},
		State:         row.State,
		OriginalName:  row.OriginalName,
		DisplayName:   row.DisplayName,
		MediaType:     row.MediaType,
		Extension:     row.Extension,
		SizeBytes:     row.SizeBytes,
		SHA256:        row.SHA256,
		InlineCapable: row.InlineCapable,
		SourceType:    row.SourceType,
		ContentURL:    s.attachmentContentURL(row.ID),
		CreatedBy:     domain.ActorInfoToJSON(actor),
		CreatedAt:     row.CreatedAt,
		ModifiedAt:    row.ModifiedAt,
	}, nil
}

func (s *Service) attachmentContentURL(id string) string {
	if s.resourceBaseURL == "" {
		return "/api/v1/attachments/" + id + "/content"
	}
	return s.resourceBaseURL + "/api/v1/attachments/" + id + "/content"
}

func (s *Service) quotaLimits() storage.AttachmentQuotaLimits {
	return storage.AttachmentQuotaLimits{
		MaxResourceTotalSizeBytes:  s.attachmentRuntime.Config.MaxResourceTotalSizeBytes,
		MaxWorkspaceTotalSizeBytes: s.attachmentRuntime.Config.MaxWorkspaceTotalSizeBytes,
		MaxAttachmentsPerResource:  s.attachmentRuntime.Config.MaxAttachmentsPerResource,
	}
}

func (s *Service) actorCreatorID() string {
	if s.runtime.ActorUserID != "" {
		return s.runtime.ActorUserID
	}
	if s.runtime.ActorTokenID != "" {
		return s.runtime.ActorTokenID
	}
	return s.runtime.ActorType
}

func (s *Service) attachmentReferencedByDescription(row storage.Attachment) (bool, error) {
	if row.AttachedToType != storage.AttachmentAttachedToTask {
		return false, nil
	}
	tsk, err := s.resolveTargetForRead(row.AttachedToID)
	if err != nil {
		return false, nil
	}
	ids, err := domain.AttachmentReferenceIDs(optionalTextValue(tsk.Description))
	if err != nil {
		return false, err
	}
	for _, id := range ids {
		if id == row.ID {
			return true, nil
		}
	}
	return false, nil
}

func (s *Service) bestEffortDeleteBlob(ctx context.Context, store blobstore.Store, key string) {
	_ = store.Delete(ctx, key)
}

func mapInspectError(err error) error {
	var ie attachments.InspectError
	if errors.As(err, &ie) {
		return RuntimeError{Code: ie.Code, Message: ie.Message}
	}
	return err
}

func mapQuotaError(err error) error {
	var qe storage.QuotaExceededError
	if errors.As(err, &qe) {
		return RuntimeError{Code: "attachment_quota_exceeded", Message: qe.Error()}
	}
	return err
}

func mapRepoNotFound(err error) error {
	if errors.Is(err, storage.ErrNotFound) {
		return RuntimeError{Code: "attachment_not_found", Message: "attachment not found"}
	}
	return err
}

// mapFetchError 把 safefetch.FetchError 转为 RuntimeError。
func mapFetchError(err error) error {
	var fe safefetch.FetchError
	if errors.As(err, &fe) {
		return RuntimeError{Code: fe.Code, Message: fe.Message}
	}
	return RuntimeError{Code: "attachment_remote_fetch_failed", Message: err.Error()}
}

// attachmentStorageKey 按 spec §9.2 规则生成 storage key。
func attachmentStorageKey(workspaceID, attachmentID string) string {
	return fmt.Sprintf("workspaces/%s/attachments/%s", workspaceID, attachmentID)
}

// strPtr 已在 task_occurrence_test.go 中定义，这里复用。

// attachmentAuditPayload 构造审计 payload，不暴露 storage key/source URL。
func attachmentAuditPayload(row storage.Attachment, target AttachmentTarget) map[string]any {
	if row.ID == "" {
		return nil
	}
	payload := map[string]any{
		"attachment_id": row.ID,
		"attached_to": map[string]string{
			"type": target.Type,
			"id":   target.ID,
		},
		"display_name": row.DisplayName,
		"media_type":   row.MediaType,
		"size_bytes":   row.SizeBytes,
		"sha256":       row.SHA256,
		"source_type":  row.SourceType,
		"source_host":  row.SourceHost,
	}
	return payload
}

// buildAttachmentRow 构造初始 uploading metadata row。
func buildAttachmentRow(id string, target AttachmentTarget, inspected attachments.InspectedFile, in AttachmentUploadInput, backend, key string, runtime RuntimeContext, now int64) storage.Attachment {
	row := storage.Attachment{
		ID:                   id,
		WorkspaceID:          target.WorkspaceID,
		AttachedToType:       target.Type,
		AttachedToID:         target.ID,
		State:                storage.AttachmentStateUploading,
		OriginalName:         inspected.OriginalName,
		DisplayName:          inspected.OriginalName,
		MediaType:            inspected.MediaType,
		Extension:            inspected.Extension,
		SizeBytes:            0, // uploading 阶段 size 还未确认
		SHA256:               inspected.SHA256,
		InlineCapable:        inspected.InlineCapable,
		SourceType:           "upload",
		StorageBackend:       backend,
		StorageKey:           key,
		CreatedAt:            now,
		ModifiedAt:           now,
		CreatedBy:            runtime.ActorUserID,
		CreatedByActorType:   actorTypeOrDefault(runtime),
		CreatedByUserID:      nilIfEmpty(runtime.ActorUserID),
		CreatedByTokenID:     nilIfEmpty(runtime.ActorTokenID),
		CreatedByTokenName:   nilIfEmpty(runtime.ActorTokenName),
		CreatedByTokenPrefix: nilIfEmpty(runtime.ActorTokenPrefix),
	}
	if row.CreatedBy == "" {
		row.CreatedBy = runtime.ActorType
	}
	return row
}

func actorTypeOrDefault(runtime RuntimeContext) string {
	if runtime.ActorType == "" {
		return "user"
	}
	return runtime.ActorType
}

func nilIfEmpty(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}

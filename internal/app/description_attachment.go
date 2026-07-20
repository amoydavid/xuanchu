package app

import (
	"context"
	"errors"
	"fmt"

	"git.dajee.net/dajee/xuanchu/internal/storage"
	domain "git.dajee.net/dajee/xuanchu/internal/task"
)

// maxDescriptionBytes 是 spec §6.3 的 description 上限。
const maxDescriptionBytes = 512 * 1024

// maxDescriptionReferences 是 spec §6.3 的内部引用数量上限。
const maxDescriptionReferences = 200

// validateAndBindDescriptionAttachments 校验 description 中的附件引用，
// 并在同一事务内激活调用者创建的相关 draft。
//
// 由 Modify / Add / import / task bundle 的 description 写路径调用。
// before 为空 task 表示新增；after 是即将落库的 task（含新 description）。
//
// 校验顺序：
//  1. description UTF-8 字节 ≤ 512 KiB
//  2. 内部引用节点 ≤ 200
//  3. 附件归属当前 task
//  4. 激活 draft
func (s *Service) validateAndBindDescriptionAttachments(before, after domain.Task) error {
	refs, err := s.validateDescriptionReferences(before.Description, after.Description, true)
	if err != nil {
		return err
	}
	if s.attachmentRuntime == nil {
		// 附件存储是可选配置。未启用时仍须保留 Markdown 中的 attachment 引用，
		// 以支持不携带二进制的导入/导出；此时不能执行归属校验或 draft 激活。
		return nil
	}

	// 校验 attachment 引用归属当前 task。
	attachmentIDs := uniqueAttachmentIDs(refs)
	for _, id := range attachmentIDs {
		row, err := s.attachmentRepo.GetByID(id)
		if err != nil {
			return RuntimeError{Code: "description_reference_invalid", Message: "attachment reference is unavailable"}
		}
		if row.WorkspaceID != s.workspaceID ||
			row.AttachedToType != "task" ||
			row.AttachedToID != after.UUID {
			return RuntimeError{Code: "description_reference_invalid", Message: "attachment reference belongs to a different task"}
		}
		// draft 是编辑会话的私有资源。即使攻击者知道 attachment UUID，也不能
		// 把其他 actor 的 draft 写进 description 后借保存路径绑定它。
		if row.State == storage.AttachmentStateDraft && row.CreatedBy != s.actorCreatorID() {
			return RuntimeError{Code: "attachment_draft_creator_mismatch", Message: "only the draft creator can bind this attachment"}
		}
	}

	// 激活调用者创建的 draft。归属校验通过后，ActivateDrafts 会进一步限制只激活
	// 当前 creator 的 draft。
	if err := s.ActivateDescriptionDrafts(after.UUID, attachmentIDs); err != nil {
		return err
	}
	return nil
}

// validateDescriptionReferences 校验所有 description 入口共享的文本与语义引用边界。
// series 不拥有 attachment target，因此 allowAttachments=false 时显式拒绝 attachment URI。
func (s *Service) validateDescriptionReferences(before, after *string, allowAttachments bool) ([]domain.ContentReference, error) {
	desc := optionalTextValue(after)
	if len(desc) > maxDescriptionBytes {
		return nil, RuntimeError{Code: "description_too_large", Message: fmt.Sprintf("description exceeds %d bytes", maxDescriptionBytes)}
	}

	refs, err := domain.ParseContentReferences(desc)
	if err != nil {
		return nil, RuntimeError{Code: "description_reference_invalid", Message: err.Error()}
	}
	if len(refs) > maxDescriptionReferences {
		return nil, RuntimeError{Code: "description_reference_limit_exceeded", Message: fmt.Sprintf("description has %d references, max %d", len(refs), maxDescriptionReferences)}
	}
	if !allowAttachments && len(uniqueAttachmentIDs(refs)) > 0 {
		return nil, RuntimeError{Code: "description_reference_invalid", Message: "attachment references are not supported for task series"}
	}
	// user/task 引用不是纯展示文本：仅校验本次新增的稳定身份，防止手写 UUID
	// 绕过 suggestion 的成员/项目权限过滤。历史引用若后来失去可读性，仍可保留
	// 并编辑无关文本；label 变化不改变稳定身份，也不应重新触发校验。
	beforeRefs, err := domain.ParseContentReferences(optionalTextValue(before))
	if err != nil {
		return nil, RuntimeError{Code: "description_reference_invalid", Message: err.Error()}
	}
	added, _ := domain.DiffContentReferenceKeys(beforeRefs, refs)
	keys := make([]ContentReferenceKeyInput, 0, len(added))
	for _, ref := range added {
		switch ref.Kind {
		case domain.ContentReferenceUser:
			keys = append(keys, ContentReferenceKeyInput{Type: "user", ID: ref.ID})
		case domain.ContentReferenceTask:
			keys = append(keys, ContentReferenceKeyInput{Type: "task", ID: ref.ID})
		}
	}
	if len(keys) > 0 {
		// 写权限不蕴含读取权限（尤其是 tenant access token）。新增引用必须
		// 在当前 request scope 可读，不能借 description 写路径探测或关联隐藏资源。
		if err := s.Require(PermissionTaskRead); err != nil {
			return nil, err
		}
		resolved, err := s.ResolveContentReferences(context.Background(), keys)
		if err != nil {
			return nil, err
		}
		for _, result := range resolved {
			if result.Status != "resolved" {
				return nil, RuntimeError{Code: "description_reference_invalid", Message: "content reference is unavailable"}
			}
		}
	}
	return refs, nil
}

func uniqueAttachmentIDs(refs []domain.ContentReference) []string {
	seen := map[string]struct{}{}
	var ids []string
	for _, r := range refs {
		if r.Kind != domain.ContentReferenceAttachment {
			continue
		}
		if _, ok := seen[r.ID]; ok {
			continue
		}
		seen[r.ID] = struct{}{}
		ids = append(ids, r.ID)
	}
	return ids
}

func (s *Service) bindTaskCreationDrafts(draftTargetID, taskID string, attachmentIDs []string) error {
	if draftTargetID == "" {
		return nil
	}
	if s.attachmentRuntime == nil {
		return RuntimeError{Code: "attachment_storage_unavailable", Message: "attachments not configured"}
	}
	if _, err := s.attachmentRepo.BindTaskDrafts(
		s.workspaceID,
		draftTargetID,
		taskID,
		s.actorCreatorID(),
		attachmentIDs,
		s.clock.Unix(),
	); err != nil {
		return err
	}
	return nil
}

// IsRuntimeErrorCode 返回 RuntimeError 的 code（如果有）。
func IsRuntimeErrorCode(err error) (string, bool) {
	var re RuntimeError
	if errors.As(err, &re) {
		return re.Code, true
	}
	return "", false
}

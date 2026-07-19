package app

import (
	"errors"
	"fmt"

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
	if s.attachmentRuntime == nil {
		// 附件运行时未启用时仍允许写入不含附件引用的 description；
		// 含附件引用则要求运行时存在。
		refs, err := domain.AttachmentReferenceIDs(optionalTextValue(after.Description))
		if err != nil {
			return err
		}
		if len(refs) == 0 {
			return nil
		}
		return RuntimeError{Code: "attachment_storage_unavailable", Message: "attachments not configured"}
	}

	desc := optionalTextValue(after.Description)
	if len(desc) > maxDescriptionBytes {
		return RuntimeError{Code: "description_too_large", Message: fmt.Sprintf("description exceeds %d bytes", maxDescriptionBytes)}
	}

	refs, err := domain.ParseContentReferences(desc)
	if err != nil {
		return RuntimeError{Code: "description_reference_invalid", Message: err.Error()}
	}
	if len(refs) > maxDescriptionReferences {
		return RuntimeError{Code: "description_reference_limit_exceeded", Message: fmt.Sprintf("description has %d references, max %d", len(refs), maxDescriptionReferences)}
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
	}

	// 激活调用者创建的 draft。归属校验通过后，ActivateDrafts 会进一步限制只激活
	// 当前 creator 的 draft。
	if err := s.ActivateDescriptionDrafts(after.UUID, attachmentIDs); err != nil {
		// ActivateDescriptionDrafts 在 creator 为空时返回 attachment_draft_creator_mismatch，
		// 但历史 description 含附件引用的场景（creator 不是当前 actor）需要允许：
		// 只要 creator 一致就激活；不一致静默跳过，不阻塞 description 写入。
		if code, ok := IsRuntimeErrorCode(err); ok && code == "attachment_draft_creator_mismatch" {
			return nil
		}
		return err
	}
	return nil
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

// IsRuntimeErrorCode 返回 RuntimeError 的 code（如果有）。
func IsRuntimeErrorCode(err error) (string, bool) {
	var re RuntimeError
	if errors.As(err, &re) {
		return re.Code, true
	}
	return "", false
}

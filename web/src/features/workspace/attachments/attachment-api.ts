import {
  workspaceApiBlob,
  workspaceApiDelete,
  workspaceApiGet,
  workspaceApiMultipart,
  workspaceApiPatch,
  workspaceApiPost,
} from "@/features/workspace/session/workspace-api"

// Attachment 对齐后端 AttachmentView JSON 形状。
export type AttachmentActorInfo = {
  type: string
  user?: {
    id: string
    name: string
    display_name?: string
    email?: string
    external_ids?: { provider: string; user_type?: string; external_id: string }[]
  }
  token?: { id: string; name: string; prefix?: string }
}

export type Attachment = {
  id: string
  attached_to: { type: "task" | "task_draft"; id: string }
  state: "uploading" | "draft" | "active" | "deleted"
  original_name: string
  display_name: string
  media_type: string
  extension: string
  size_bytes: number
  sha256: string
  inline_capable: boolean
  source_type: "upload" | "remote_url"
  content_url: string
  created_by: AttachmentActorInfo
  created_at: number
  modified_at: number
}

export type AttachmentUploadMode = "attachment" | "description_draft"

export type AttachmentUploadResult = {
  attachment: Attachment
}

function encodeSegment(value: string): string {
  return encodeURIComponent(value)
}

function workspaceQuery(workspaceSlug: string): string {
  return `workspace=${encodeURIComponent(workspaceSlug)}`
}

export function taskAttachmentsPath(
  workspaceSlug: string,
  taskRef: string,
  options?: { includeDrafts?: boolean }
): string {
  const query = workspaceQuery(workspaceSlug)
  if (options?.includeDrafts) {
    return `/api/v1/tasks/${encodeSegment(taskRef)}/attachments?${query}&include_drafts=true`
  }
  return `/api/v1/tasks/${encodeSegment(taskRef)}/attachments?${query}`
}

export function taskAttachmentImportURLPath(
  workspaceSlug: string,
  taskRef: string
): string {
  return `/api/v1/tasks/${encodeSegment(taskRef)}/attachments/import-url?${workspaceQuery(workspaceSlug)}`
}

export function taskDraftAttachmentsPath(
  workspaceSlug: string,
  draftTarget: string
): string {
  return `/api/v1/task-drafts/${encodeSegment(draftTarget)}/attachments?${workspaceQuery(workspaceSlug)}`
}

export function taskDraftAttachmentImportURLPath(
  workspaceSlug: string,
  draftTarget: string
): string {
  return `/api/v1/task-drafts/${encodeSegment(draftTarget)}/attachments/import-url?${workspaceQuery(workspaceSlug)}`
}

export function attachmentItemPath(
  workspaceSlug: string,
  attachmentID: string
): string {
  return `/api/v1/attachments/${encodeSegment(attachmentID)}?${workspaceQuery(workspaceSlug)}`
}

export function attachmentContentPath(
  workspaceSlug: string,
  attachmentID: string
): string {
  return `/api/v1/attachments/${encodeSegment(attachmentID)}/content?${workspaceQuery(workspaceSlug)}`
}

// listTaskAttachments 列出 task 的 active 附件。
export function listTaskAttachments(
  workspaceSlug: string,
  taskRef: string
): Promise<Attachment[]> {
  return workspaceApiGet<Attachment[]>(taskAttachmentsPath(workspaceSlug, taskRef))
}

// getAttachment 返回附件 metadata。
export function getAttachment(
  workspaceSlug: string,
  attachmentID: string
): Promise<Attachment> {
  return workspaceApiGet<Attachment>(attachmentItemPath(workspaceSlug, attachmentID))
}

// getAttachmentBlob 流式下载附件二进制内容。
export function getAttachmentBlob(
  workspaceSlug: string,
  attachmentID: string,
  signal?: AbortSignal
): Promise<Blob> {
  return workspaceApiBlob(attachmentContentPath(workspaceSlug, attachmentID), signal)
}

// uploadTaskAttachment 上传 multipart 附件。
export function uploadTaskAttachment(
  workspaceSlug: string,
  taskRef: string,
  input: {
    file: File
    mode: AttachmentUploadMode
    displayName?: string
  },
  options?: { signal?: AbortSignal; onProgress?: (sent: number, total: number) => void }
): Promise<Attachment> {
  const form = new FormData()
  form.append("file", input.file)
  form.append("mode", input.mode)
  if (input.displayName) {
    form.append("display_name", input.displayName)
  }
  return workspaceApiMultipart<Attachment>(
    taskAttachmentsPath(workspaceSlug, taskRef),
    form,
    options
  )
}

// importTaskAttachmentURL 远程图片转存。
export function importTaskAttachmentURL(
  workspaceSlug: string,
  taskRef: string,
  input: { sourceURL: string; mode: AttachmentUploadMode; displayName?: string },
  options?: { signal?: AbortSignal }
): Promise<Attachment> {
  return workspaceApiPost<Attachment>(
    taskAttachmentImportURLPath(workspaceSlug, taskRef),
    {
      source_url: input.sourceURL,
      mode: input.mode,
      display_name: input.displayName,
    },
    options
  )
}

// uploadTaskDraftAttachment 在任务尚未创建时，把截图暂存到本次创建专属的私有 target。
export function uploadTaskDraftAttachment(
  workspaceSlug: string,
  draftTarget: string,
  input: { file: File; mode: AttachmentUploadMode; displayName?: string },
  options?: { signal?: AbortSignal; onProgress?: (sent: number, total: number) => void }
): Promise<Attachment> {
  const form = new FormData()
  form.append("file", input.file)
  if (input.displayName) form.append("display_name", input.displayName)
  return workspaceApiMultipart<Attachment>(taskDraftAttachmentsPath(workspaceSlug, draftTarget), form, options)
}

// importTaskDraftAttachmentURL 将远程图片转存到任务创建前的私有 draft target。
export function importTaskDraftAttachmentURL(
  workspaceSlug: string,
  draftTarget: string,
  input: { sourceURL: string; displayName?: string },
  options?: { signal?: AbortSignal }
): Promise<Attachment> {
  return workspaceApiPost<Attachment>(
    taskDraftAttachmentImportURLPath(workspaceSlug, draftTarget),
    {
      source_url: input.sourceURL,
      display_name: input.displayName,
    },
    options
  )
}

// renameAttachment 修改附件展示名。
export function renameAttachment(
  workspaceSlug: string,
  attachmentID: string,
  displayName: string
): Promise<Attachment> {
  return workspaceApiPatch<Attachment>(
    attachmentItemPath(workspaceSlug, attachmentID),
    { display_name: displayName }
  )
}

// removeAttachment 删除附件。
export function removeAttachment(
  workspaceSlug: string,
  attachmentID: string
): Promise<void> {
  return workspaceApiDelete<void>(attachmentItemPath(workspaceSlug, attachmentID))
}

// formatAttachmentSize 把字节数格式化为可读字符串。
export function formatAttachmentSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KiB`
  if (bytes < 1024 * 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(1)} MiB`
  return `${(bytes / (1024 * 1024 * 1024)).toFixed(1)} GiB`
}

// 防止 tree-shaking 误删未直接调用但导出的 helper。
export type AttachmentUploadInput = {
  file: File
  mode: AttachmentUploadMode
  displayName?: string
}
export type AttachmentUploadResultLegacy = AttachmentUploadResult

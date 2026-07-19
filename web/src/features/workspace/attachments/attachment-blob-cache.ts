import type { Attachment } from "./attachment-api"
import { getAttachmentBlob } from "./attachment-api"

// CacheEntry 描述一个 attachment 的 object URL 与引用计数。
type CacheEntry = {
  promise: Promise<string>
  refs: number
  objectURL?: string
}

// AttachmentBlobKey 是 cache 的稳定键：attachment id + sha256，避免同一图片被多次抓取。
export type AttachmentBlobKey = string

function attachmentBlobKey(attachment: Pick<Attachment, "id" | "sha256">): AttachmentBlobKey {
  return `${attachment.id}:${attachment.sha256}`
}

const cache = new Map<AttachmentBlobKey, CacheEntry>()

// resetAttachmentBlobCache 清空整个 cache，主要用于 session/workspace 变化或测试。
export function resetAttachmentBlobCache(): void {
  for (const entry of cache.values()) {
    if (entry.objectURL) {
      URL.revokeObjectURL(entry.objectURL)
    }
  }
  cache.clear()
}

// AcquireAttachmentBlobResult 是 acquireAttachmentBlob 的返回值。
export type AcquireAttachmentBlobResult = {
  url: string
  release: () => void
}

// acquireAttachmentBlob 获取 attachment 内容的 object URL，并对同一图片去重。
//
// 调用方在组件卸载、引用变化或 session 变化时必须调用 release()，
// 最后一个 consumer release 时才会真正 revoke。
export async function acquireAttachmentBlob(
  workspaceSlug: string,
  attachment: Pick<Attachment, "id" | "sha256">,
  signal?: AbortSignal
): Promise<AcquireAttachmentBlobResult> {
  const key = attachmentBlobKey(attachment)
  let entry = cache.get(key)
  if (!entry) {
    entry = {
      promise: fetchAttachmentObjectURL(workspaceSlug, attachment.id, signal),
      refs: 0,
    }
    cache.set(key, entry)
  }
  entry.refs += 1
  const url = await entry.promise
  if (!entry.objectURL) {
    entry.objectURL = url
  }
  let released = false
  return {
    url,
    release: () => {
      if (released) return
      released = true
      const current = cache.get(key)
      if (!current) return
      current.refs -= 1
      if (current.refs <= 0) {
        if (current.objectURL) {
          URL.revokeObjectURL(current.objectURL)
        }
        cache.delete(key)
      }
    },
  }
}

async function fetchAttachmentObjectURL(
  workspaceSlug: string,
  attachmentID: string,
  signal?: AbortSignal
): Promise<string> {
  const blob = await getAttachmentBlob(workspaceSlug, attachmentID, signal)
  return URL.createObjectURL(blob)
}

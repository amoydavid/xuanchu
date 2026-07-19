import type { Attachment } from "@/features/workspace/attachments"

// UploadQueueItemStatus 描述队列项的状态机。
export type UploadQueueItemStatus =
  | "queued"
  | "uploading"
  | "resolved"
  | "failed"
  | "cancelled"

// UploadQueueItemKind 描述候选项类型：file/data/remote。
export type UploadQueueItemKind = "file" | "data" | "remote"

// UploadQueueCandidate 是入队的候选项。
export type UploadQueueCandidate = {
  kind: UploadQueueItemKind
  // file/data 模式的源文件；remote 模式留空。
  file?: File
  // remote 模式的 source URL；file/data 留空。
  sourceURL?: string
  alt: string
}

// UploadQueueItem 是队列中的单项状态。
export type UploadQueueItem = {
  key: string
  candidate: UploadQueueCandidate
  status: UploadQueueItemStatus
  progress: number
  attachment?: Attachment
  error?: { code: string; message: string }
}

// AttachmentUploadQueueOptions 描述队列配置。
export type AttachmentUploadQueueOptions = {
  remoteConcurrency?: number
  onProgress?: (key: string, progress: number) => void
}

// AttachmentUploadQueueAPI 是队列对外的最小接口，便于测试注入。
export type AttachmentUploadQueueAPI = {
  uploadFile(taskRef: string, candidate: UploadQueueCandidate, signal: AbortSignal): Promise<Attachment>
  importRemoteURL(taskRef: string, sourceURL: string, signal: AbortSignal): Promise<Attachment>
  removeDraft(attachmentID: string, signal: AbortSignal): Promise<void>
}

// AttachmentUploadQueue 管理粘贴/拖入/远程转存的上传流程。
//
// spec §5.2 行为：
// - 同一次粘贴按规范化后的 source URL 去重，前端最多并发 3 个远程抓取。
// - 每项持有 AbortController；cancelAll 取消未完成请求。
// - cleanupDrafts 对已成功但未保存的 draft 调用 DELETE。
// - 一个失败不取消其他项。
export class AttachmentUploadQueue {
  private items = new Map<string, UploadQueueItem>()
  private controllers = new Map<string, AbortController>()
  private remotePromises = new Map<string, Promise<Attachment>>()
  private remoteSemaphore: Promise<void> = Promise.resolve()
  private remoteActive = 0
  private remoteConcurrency: number
  private onProgress?: (key: string, progress: number) => void
  private api: AttachmentUploadQueueAPI

  constructor(api: AttachmentUploadQueueAPI, opts: AttachmentUploadQueueOptions = {}) {
    this.api = api
    this.remoteConcurrency = opts.remoteConcurrency ?? 3
    this.onProgress = opts.onProgress
  }

  // enqueue 把候选项加入队列并开始上传。
  enqueue(candidate: UploadQueueCandidate): Promise<Attachment> {
    const key = makeKey(candidate)
    // 远程 URL 去重：同一次粘贴内复用一个 attachment。
    if (candidate.kind === "remote" && candidate.sourceURL) {
      const normalized = normalizeRemoteURL(candidate.sourceURL)
      const existing = this.remotePromises.get(normalized)
      if (existing) {
        return existing
      }
    }
    const controller = new AbortController()
    this.controllers.set(key, controller)
    const item: UploadQueueItem = {
      key,
      candidate,
      status: "queued",
      progress: 0,
    }
    this.items.set(key, item)
    const promise = this.runItem(key, candidate, controller.signal)
    if (candidate.kind === "remote" && candidate.sourceURL) {
      this.remotePromises.set(normalizeRemoteURL(candidate.sourceURL), promise)
    }
    return promise
  }

  private async runItem(
    key: string,
    candidate: UploadQueueCandidate,
    signal: AbortSignal
  ): Promise<Attachment> {
    try {
      this.update(key, { status: "uploading", progress: 0 })
      // remote 需要按 concurrency 限流。
      let taskRef = "" // 调用方需要通过 setTaskRef 注入；占位由 uploadFile/importRemoteURL 处理。
      const task = this.taskRef
      taskRef = task
      let attachment: Attachment
      if (candidate.kind === "remote" && candidate.sourceURL) {
        attachment = await this.runWithConcurrency(signal, () =>
          this.api.importRemoteURL(taskRef, candidate.sourceURL!, signal)
        )
      } else if (candidate.file) {
        attachment = await this.api.uploadFile(taskRef, candidate, signal)
      } else {
        throw new Error("invalid candidate")
      }
      this.update(key, { status: "resolved", progress: 100, attachment })
      return attachment
    } catch (err) {
      if (signal.aborted) {
        this.update(key, { status: "cancelled" })
      } else {
        const e = err as { code?: string; message?: string }
        this.update(key, {
          status: "failed",
          error: { code: e?.code ?? "unknown", message: e?.message ?? "failed" },
        })
      }
      throw err
    }
  }

  // taskRef 由队列持有者注入。
  taskRef = ""

  // setTaskRef 设置队列的 taskRef（队列初始化时由调用方设置）。
  setTaskRef(taskRef: string) {
    this.taskRef = taskRef
  }

  private async runWithConcurrency<T>(
    signal: AbortSignal,
    fn: () => Promise<T>
  ): Promise<T> {
    // 简单信号量：等待空闲槽。
    while (this.remoteActive >= this.remoteConcurrency) {
      await this.remoteSemaphore
    }
    this.remoteActive += 1
    let resolveRelease: () => void = () => {}
    this.remoteSemaphore = new Promise((resolve) => {
      resolveRelease = () => {
        this.remoteActive -= 1
        resolve()
      }
    })
    try {
      if (signal.aborted) throw new DOMException("aborted", "AbortError")
      return await fn()
    } finally {
      resolveRelease()
    }
  }

  private update(key: string, patch: Partial<UploadQueueItem>) {
    const item = this.items.get(key)
    if (!item) return
    Object.assign(item, patch)
    this.onProgress?.(key, item.progress)
  }

  // cancelAll 取消所有未完成的上传。
  cancelAll() {
    for (const controller of this.controllers.values()) {
      controller.abort()
    }
    this.controllers.clear()
  }

  // cleanupDrafts 删除已成功但未保存（resolved 状态）的 draft。
  async cleanupDrafts(): Promise<void> {
    const tasks: Promise<void>[] = []
    for (const item of this.items.values()) {
      if (item.status === "resolved" && item.attachment) {
        const controller = new AbortController()
        tasks.push(
          this.api.removeDraft(item.attachment.id, controller.signal).catch(() => {
            // best-effort：删除失败留给 janitor 兜底。
          })
        )
      }
    }
    await Promise.all(tasks)
  }

  // getItems 返回当前所有队列项的快照。
  getItems(): UploadQueueItem[] {
    return Array.from(this.items.values())
  }

  // getItem 按 key 返回单项。
  getItem(key: string): UploadQueueItem | undefined {
    return this.items.get(key)
  }
}

// makeKey 为候选项生成稳定 key。
export function makeKey(candidate: UploadQueueCandidate): string {
  if (candidate.kind === "remote" && candidate.sourceURL) {
    return `remote:${normalizeRemoteURL(candidate.sourceURL)}`
  }
  if (candidate.file) {
    return `file:${candidate.file.name}:${candidate.file.size}:${candidate.file.lastModified}`
  }
  return `data:${candidate.alt}`
}

// normalizeRemoteURL 规范化远程 URL：scheme/host 小写、移除 fragment、保留 path/query。
export function normalizeRemoteURL(raw: string): string {
  try {
    const u = new URL(raw)
    u.hash = ""
    u.host = u.host.toLowerCase()
    u.protocol = u.protocol.toLowerCase()
    return u.toString()
  } catch {
    return raw
  }
}

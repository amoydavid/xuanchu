import { afterEach, describe, expect, it, vi } from "vitest"

import type { Attachment } from "@/features/workspace/attachments"

import {
  AttachmentUploadQueue,
  type AttachmentUploadQueueAPI,
  type UploadQueueCandidate,
} from "./attachment-upload-queue"

function makeAttachment(id: string): Attachment {
  return {
    id,
    attached_to: { type: "task", id: "t1" },
    state: "draft",
    original_name: "x.png",
    display_name: "x",
    media_type: "image/png",
    extension: ".png",
    size_bytes: 1,
    sha256: "h",
    inline_capable: true,
    source_type: "upload",
    content_url: `/api/v1/attachments/${id}/content`,
    created_by: { type: "user", user: { id: "u", name: "u" } },
    created_at: 1,
    modified_at: 1,
  }
}

function mockApi(): AttachmentUploadQueueAPI & {
  uploadFile: ReturnType<typeof vi.fn>
  importRemoteURL: ReturnType<typeof vi.fn>
  removeDraft: ReturnType<typeof vi.fn>
} {
  return {
    uploadFile: vi.fn(async () => makeAttachment("f1")),
    importRemoteURL: vi.fn(async () => makeAttachment("r1")),
    removeDraft: vi.fn(async () => {}),
  } as unknown as AttachmentUploadQueueAPI & {
    uploadFile: ReturnType<typeof vi.fn>
    importRemoteURL: ReturnType<typeof vi.fn>
    removeDraft: ReturnType<typeof vi.fn>
  }
}

afterEach(() => {
  vi.restoreAllMocks()
})

describe("AttachmentUploadQueue", () => {
  it("uploads file candidate", async () => {
    const api = mockApi()
    const queue = new AttachmentUploadQueue(api, {})
    queue.setTaskRef("t1")
    const file = new File([new Uint8Array([1])], "a.png", { type: "image/png" })
    const cand: UploadQueueCandidate = { kind: "file", file, alt: "a" }
    const attachment = await queue.enqueue(cand)
    expect(attachment.id).toBe("f1")
    expect(api.uploadFile).toHaveBeenCalledWith("t1", cand, expect.any(AbortSignal))
  })

  it("deduplicates repeated remote URLs within one paste", async () => {
    const api = mockApi()
    const queue = new AttachmentUploadQueue(api, { remoteConcurrency: 3 })
    queue.setTaskRef("t1")
    const urlA = "https://cdn.example.com/a.png"
    const urlB = "https://cdn.example.com/b.png"
    const candidates: UploadQueueCandidate[] = [
      { kind: "remote", sourceURL: urlA, alt: "A" },
      { kind: "remote", sourceURL: urlA, alt: "A2" }, // 重复
      { kind: "remote", sourceURL: urlB, alt: "B" },
    ]
    const results = await Promise.all(candidates.map((c) => queue.enqueue(c)))
    // importRemoteURL 应该只被调用 2 次（A 和 B）。
    expect(api.importRemoteURL).toHaveBeenCalledTimes(2)
    // 两个 urlA 候选应该返回同一 attachment。
    expect(results[0]?.id).toBe(results[1]?.id)
  })

  it("normalizes remote url before deduping", async () => {
    const api = mockApi()
    const queue = new AttachmentUploadQueue(api, {})
    queue.setTaskRef("t1")
    await Promise.all([
      queue.enqueue({ kind: "remote", sourceURL: "HTTPS://Example.com/a.png#frag", alt: "x" }),
      queue.enqueue({ kind: "remote", sourceURL: "https://example.com/a.png", alt: "y" }),
    ])
    expect(api.importRemoteURL).toHaveBeenCalledTimes(1)
  })

  it("limits remote concurrency to configured value", async () => {
    const api = mockApi()
    let active = 0
    let maxActive = 0
    api.importRemoteURL.mockImplementation(async () => {
      active += 1
      maxActive = Math.max(maxActive, active)
      await new Promise((resolve) => setTimeout(resolve, 50))
      active -= 1
      return makeAttachment("x")
    })
    const queue = new AttachmentUploadQueue(api, { remoteConcurrency: 3 })
    queue.setTaskRef("t1")
    const urls = ["https://x.com/1.png", "https://x.com/2.png", "https://x.com/3.png", "https://x.com/4.png"]
    await Promise.all(urls.map((u) => queue.enqueue({ kind: "remote", sourceURL: u, alt: "x" })))
    expect(maxActive).toBeLessThanOrEqual(3)
  })

  it("cancelAll aborts in-flight uploads", async () => {
    const api = mockApi()
    api.importRemoteURL.mockImplementation((_task, _url, signal: AbortSignal) => {
      return new Promise((_resolve, reject) => {
        signal.addEventListener("abort", () => {
          reject(new DOMException("aborted", "AbortError"))
        })
      })
    })
    const queue = new AttachmentUploadQueue(api, {})
    queue.setTaskRef("t1")
    const promise = queue.enqueue({
      kind: "remote",
      sourceURL: "https://x.com/1.png",
      alt: "x",
    })
    queue.cancelAll()
    await expect(promise).rejects.toThrow()
    const item = queue.getItems()[0]
    expect(item?.status).toBe("cancelled")
  })

  it("removes a late server success after cancellation", async () => {
    const api = mockApi()
    let resolveUpload: ((attachment: ReturnType<typeof makeAttachment>) => void) | undefined
    api.uploadFile.mockImplementation(() => new Promise((resolve) => { resolveUpload = resolve }))
    const queue = new AttachmentUploadQueue(api, {})
    queue.setTaskRef("t1")
    const promise = queue.enqueue({ kind: "file", file: new File(["x"], "a.png"), alt: "a" })
    queue.cancelAll()
    resolveUpload?.(makeAttachment("late"))
    await expect(promise).rejects.toThrow()
    expect(api.removeDraft).toHaveBeenCalledWith("late", expect.any(AbortSignal))
  })

  it("cleanupDrafts removes resolved drafts", async () => {
    const api = mockApi()
    const queue = new AttachmentUploadQueue(api, {})
    queue.setTaskRef("t1")
    await queue.enqueue({ kind: "file", file: new File([new Uint8Array([1])], "a.png"), alt: "a" })
    await queue.cleanupDrafts()
    expect(api.removeDraft).toHaveBeenCalledTimes(1)
  })

  it("one failure does not cancel others", async () => {
    const api = mockApi()
    let callCount = 0
    api.importRemoteURL.mockImplementation(async () => {
      callCount += 1
      if (callCount === 1) throw new Error("boom")
      return makeAttachment("ok")
    })
    const queue = new AttachmentUploadQueue(api, {})
    queue.setTaskRef("t1")
    const a = queue.enqueue({ kind: "remote", sourceURL: "https://x.com/fail.png", alt: "f" })
    const b = queue.enqueue({ kind: "remote", sourceURL: "https://x.com/ok.png", alt: "o" })
    await expect(a).rejects.toThrow()
    await expect(b).resolves.toBeTruthy()
    const failed = queue.getItem("remote:https://x.com/fail.png")
    expect(failed?.status).toBe("failed")
  })
})

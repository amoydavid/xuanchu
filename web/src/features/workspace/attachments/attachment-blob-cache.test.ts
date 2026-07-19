import { afterEach, describe, expect, it, vi } from "vitest"

import { ApiError } from "@/lib/api"
import * as attachmentApi from "./attachment-api"
import {
  acquireAttachmentBlob,
  resetAttachmentBlobCache,
} from "./attachment-blob-cache"

const createObjectURL = vi.fn(() => "blob:test")
const revokeObjectURL = vi.fn()
Object.defineProperty(globalThis, "URL", {
  value: {
    ...globalThis.URL,
    createObjectURL,
    revokeObjectURL,
  },
  configurable: true,
})

afterEach(() => {
  vi.restoreAllMocks()
  resetAttachmentBlobCache()
  createObjectURL.mockClear()
  revokeObjectURL.mockClear()
})

describe("attachment-blob-cache", () => {
  it("deduplicates repeated requests by id+sha256", async () => {
    const blobSpy = vi
      .spyOn(attachmentApi, "getAttachmentBlob")
      .mockResolvedValue(new Blob(["payload"]))

    const attachment = { id: "att-1", sha256: "h1" }
    const a = await acquireAttachmentBlob("dajee", attachment)
    const b = await acquireAttachmentBlob("dajee", attachment)
    expect(blobSpy).toHaveBeenCalledTimes(1)
    expect(a.url).toBe("blob:test")
    expect(b.url).toBe("blob:test")
    a.release()
    expect(revokeObjectURL).not.toHaveBeenCalled()
    b.release()
    expect(revokeObjectURL).toHaveBeenCalledWith("blob:test")
  })

  it("re-fetches after cache is released and re-acquired", async () => {
    const blobSpy = vi
      .spyOn(attachmentApi, "getAttachmentBlob")
      .mockResolvedValue(new Blob(["payload"]))

    const attachment = { id: "att-2", sha256: "h2" }
    const first = await acquireAttachmentBlob("dajee", attachment)
    first.release()
    const second = await acquireAttachmentBlob("dajee", attachment)
    second.release()
    expect(blobSpy).toHaveBeenCalledTimes(2)
  })

  it("propagates load errors and clears cache entry", async () => {
    const blobSpy = vi
      .spyOn(attachmentApi, "getAttachmentBlob")
      .mockRejectedValue(new ApiError(410, "attachment_content_gone", "gone"))
    const attachment = { id: "att-3", sha256: "h3" }
    await expect(acquireAttachmentBlob("dajee", attachment)).rejects.toBeInstanceOf(ApiError)
    expect(blobSpy).toHaveBeenCalledTimes(1)
  })
})

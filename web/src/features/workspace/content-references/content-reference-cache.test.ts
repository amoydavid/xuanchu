import { afterEach, describe, expect, it, vi } from "vitest"

import * as api from "./content-reference-api"
import {
  loadContentReference,
  resetContentReferenceLoader,
} from "./content-reference-cache"

afterEach(() => {
  vi.restoreAllMocks()
  resetContentReferenceLoader()
})

describe("content-reference-cache", () => {
  it("batches mixed references once and preserves unavailable", async () => {
    const resolveSpy = vi
      .spyOn(api, "resolveContentReferences")
      .mockResolvedValue([
        { type: "user", id: "u1", status: "resolved", user: { id: "u1", name: "alice" } },
        { type: "attachment", id: "a1", status: "resolved", attachment: {
          id: "a1", display_name: "x.png", media_type: "image/png",
          size_bytes: 1, inline_capable: true, content_url: "/c"
        } },
      ])

    const [user, task, image] = await Promise.all([
      loadContentReference({ type: "user", id: "u1" }),
      loadContentReference({ type: "task", id: "t1" }),
      loadContentReference({ type: "attachment", id: "a1" }),
    ])
    expect(resolveSpy).toHaveBeenCalledOnce()
    expect(user.status).toBe("resolved")
    expect(task.status).toBe("unavailable")
    expect(image.status).toBe("resolved")
  })

  it("deduplicates same key within a batch", async () => {
    const resolveSpy = vi
      .spyOn(api, "resolveContentReferences")
      .mockResolvedValue([
        { type: "user", id: "u1", status: "resolved", user: { id: "u1", name: "alice" } },
      ])
    const [a, b] = await Promise.all([
      loadContentReference({ type: "user", id: "u1" }),
      loadContentReference({ type: "user", id: "u1" }),
    ])
    expect(resolveSpy).toHaveBeenCalledOnce()
    expect(a.status).toBe("resolved")
    expect(b.status).toBe("resolved")
  })
})

import { afterEach, describe, expect, it, vi } from "vitest"

import { render, screen } from "@testing-library/react"
import React from "react"

import { ApiError } from "@/lib/api"
import * as blobCache from "./attachment-blob-cache"
import { AuthenticatedAttachmentImage } from "./authenticated-attachment-image"

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string) => key,
    i18n: { language: "zh-CN" },
  }),
}))

afterEach(() => {
  vi.restoreAllMocks()
})

describe("AuthenticatedAttachmentImage", () => {
  it("renders skeleton while loading and swaps to img on success", async () => {
    const acquireSpy = vi
      .spyOn(blobCache, "acquireAttachmentBlob")
      .mockResolvedValue({
        url: "blob:test",
        release: vi.fn(),
      })

    const attachment = { id: "att-1", sha256: "h1", display_name: "架构图" }
    const { rerender } = render(
      <AuthenticatedAttachmentImage
        workspaceSlug="dajee"
        attachment={attachment}
        alt="架构图"
      />
    )
    expect(acquireSpy).toHaveBeenCalledOnce()
    // 加载完成后的 img
    const img = await screen.findByRole("img", { name: "架构图" })
    expect(img.getAttribute("src")).toBe("blob:test")
    rerender(
      <AuthenticatedAttachmentImage
        workspaceSlug="dajee"
        attachment={attachment}
        alt="架构图"
      />
    )
  })

  it("renders fallback on load failure", async () => {
    vi.spyOn(blobCache, "acquireAttachmentBlob").mockRejectedValue(
      new ApiError(410, "attachment_content_gone", "attachment_content_gone")
    )
    const attachment = { id: "att-2", sha256: "h2", display_name: "旧图" }
    render(
      <AuthenticatedAttachmentImage
        workspaceSlug="dajee"
        attachment={attachment}
        alt="旧图"
      />
    )
    expect(await screen.findByText("task.attachments.content_gone")).toBeTruthy()
  })
})

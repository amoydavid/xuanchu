import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"

import { AttachmentInlineView } from "./attachment-node-view"
import * as attachments from "@/features/workspace/attachments"

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}))

vi.mock("@/features/workspace/attachments", async () => {
  const actual = await vi.importActual<typeof import("@/features/workspace/attachments")>(
    "@/features/workspace/attachments"
  )
  return {
    ...actual,
    getAttachment: vi.fn(),
  }
})

afterEach(() => {
  vi.restoreAllMocks()
})

describe("AttachmentInlineView", () => {
  it("downloads a file attachment through the authenticated blob API instead of a bare content URL", async () => {
    vi.mocked(attachments.getAttachment).mockResolvedValue({
      id: "a801f977-c745-4f47-95a4-7893a9317aba",
      sha256: "hash",
      display_name: "需求说明.pdf",
      media_type: "application/pdf",
      size_bytes: 128,
      inline_capable: false,
      content_url: "/api/v1/attachments/a801/content",
    } as never)

    render(
      <QueryClientProvider client={new QueryClient()}>
        <AttachmentInlineView
          workspaceSlug="local"
          taskRef="task-1"
          attrs={{ id: "a801f977-c745-4f47-95a4-7893a9317aba", label: "需求说明.pdf" }}
        />
      </QueryClientProvider>
    )

    expect(
      await screen.findByRole("button", { name: "需求说明.pdf" })
    ).toBeTruthy()
    expect(screen.queryByRole("link", { name: "需求说明.pdf" })).toBeNull()
  })
})

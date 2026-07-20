import { describe, expect, it } from "vitest"

import type { Attachment } from "./attachment-api"
import {
  embeddedImageAttachmentIDs,
  standaloneAttachments,
} from "./task-attachment-panel"

function attachment(
  id: string,
  inlineCapable: boolean
): Attachment {
  return {
    id,
    attached_to: { type: "task", id: "task-1" },
    state: "active",
    original_name: `${id}.png`,
    display_name: `${id}.png`,
    media_type: inlineCapable ? "image/png" : "application/pdf",
    extension: inlineCapable ? "png" : "pdf",
    size_bytes: 1,
    sha256: id,
    inline_capable: inlineCapable,
    source_type: "upload",
    content_url: "",
    created_by: { type: "user" },
    created_at: 1,
    modified_at: 1,
  }
}

describe("task attachment panel", () => {
  it("hides only inline-capable attachments rendered as description images", () => {
    const embedded = embeddedImageAttachmentIDs(
      "![架构图](ref://attachment/image-in-description)\n[文件链接](ref://attachment/file-link)"
    )
    const visible = standaloneAttachments(
      [
        attachment("image-in-description", true),
        attachment("image-uploaded-only", true),
        attachment("file-link", false),
      ],
      embedded
    )

    expect(visible.map((item) => item.id)).toEqual([
      "image-uploaded-only",
      "file-link",
    ])
  })
})

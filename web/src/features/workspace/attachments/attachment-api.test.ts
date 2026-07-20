import { describe, expect, it, vi } from "vitest"

import * as workspaceApi from "@/features/workspace/session/workspace-api"
import { importTaskAttachmentURL, importTaskDraftAttachmentURL } from "./index"

import {
  attachmentContentPath,
  attachmentItemPath,
  formatAttachmentSize,
  taskDraftAttachmentImportURLPath,
  taskDraftAttachmentsPath,
  taskAttachmentImportURLPath,
  taskAttachmentsPath,
} from "./attachment-api"

describe("attachment-api path builders", () => {
  it("encodes taskRef and workspace", () => {
    expect(taskAttachmentsPath("dajee", "occ:series:1")).toBe(
      "/api/v1/tasks/occ%3Aseries%3A1/attachments?workspace=dajee"
    )
  })

  it("appends include_drafts when requested", () => {
    expect(taskAttachmentsPath("dajee", "task-1", { includeDrafts: true })).toBe(
      "/api/v1/tasks/task-1/attachments?workspace=dajee&include_drafts=true"
    )
  })

  it("encodes attachment id and workspace for item/content", () => {
    expect(attachmentItemPath("dajee", "att-1")).toBe(
      "/api/v1/attachments/att-1?workspace=dajee"
    )
    expect(attachmentContentPath("dajee", "att-1")).toBe(
      "/api/v1/attachments/att-1/content?workspace=dajee"
    )
  })

  it("encodes import-url path", () => {
    expect(taskAttachmentImportURLPath("dajee", "task-1")).toBe(
      "/api/v1/tasks/task-1/attachments/import-url?workspace=dajee"
    )
  })

  it("places task draft import-url before the workspace query", () => {
    expect(taskDraftAttachmentsPath("dajee", "draft:1")).toBe(
      "/api/v1/task-drafts/draft%3A1/attachments?workspace=dajee"
    )
    expect(taskDraftAttachmentImportURLPath("dajee", "draft:1")).toBe(
      "/api/v1/task-drafts/draft%3A1/attachments/import-url?workspace=dajee"
    )
  })
})

describe("formatAttachmentSize", () => {
  it("formats bytes / KiB / MiB / GiB", () => {
    expect(formatAttachmentSize(0)).toBe("0 B")
    expect(formatAttachmentSize(1023)).toBe("1023 B")
    expect(formatAttachmentSize(1024)).toBe("1.0 KiB")
    expect(formatAttachmentSize(1024 * 1024)).toBe("1.0 MiB")
    expect(formatAttachmentSize(1024 * 1024 * 1024)).toBe("1.0 GiB")
  })
})

describe("attachment-api CRUD delegates to workspace helpers", () => {
  it("exports task draft import from the public attachment module", () => {
    expect(importTaskDraftAttachmentURL).toBeTypeOf("function")
  })

  it("upload uses multipart helper", async () => {
    const multipartSpy = vi
      .spyOn(workspaceApi, "workspaceApiMultipart")
      .mockResolvedValue({ id: "x" } as never)
    const { uploadTaskAttachment } = await import("./attachment-api")
    const file = new File(["a"], "a.png", { type: "image/png" })
    await uploadTaskAttachment("dajee", "task-1", { file, mode: "attachment" })
    expect(multipartSpy).toHaveBeenCalledOnce()
    vi.restoreAllMocks()
  })

  it("passes cancellation through for remote image imports", async () => {
    const postSpy = vi
      .spyOn(workspaceApi, "workspaceApiPost")
      .mockResolvedValue({ id: "x" } as never)
    const controller = new AbortController()
    await importTaskAttachmentURL(
      "dajee",
      "task-1",
      { sourceURL: "https://cdn.example.test/diagram.png", mode: "description_draft" },
      { signal: controller.signal }
    )
    expect(postSpy).toHaveBeenCalledWith(
      taskAttachmentImportURLPath("dajee", "task-1"),
      expect.objectContaining({ source_url: "https://cdn.example.test/diagram.png" }),
      { signal: controller.signal }
    )
  })
})

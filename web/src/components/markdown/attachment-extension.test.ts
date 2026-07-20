import { describe, expect, it } from "vitest"

import {
  parseAttachmentRef,
  serializeAttachmentMarkdown,
  type XuanchuAttachmentAttrs,
} from "./attachment-extension"
import { markdownManager, parseMarkdownToJSON } from "./extensions"

describe("parseAttachmentRef", () => {
  it("accepts canonical lowercase uuid", () => {
    expect(
      parseAttachmentRef("ref://attachment/40af0185-316f-42bb-b52b-545d21f6f012")
    ).toEqual({ id: "40af0185-316f-42bb-b52b-545d21f6f012", image: false })
  })

  it("returns null for non-attachment ref", () => {
    expect(parseAttachmentRef("https://example.com/a.png")).toBeNull()
    expect(parseAttachmentRef("ref://user/40af0185-316f-42bb-b52b-545d21f6f012")).toBeNull()
  })

  it("throws on malformed ref://attachment", () => {
    expect(() => parseAttachmentRef("ref://attachment/not-a-uuid")).toThrow(
      "description_reference_invalid"
    )
    expect(() =>
      parseAttachmentRef("ref://attachment/40af0185-316f-42bb-b52b-545d21f6f012/extra")
    ).toThrow("description_reference_invalid")
  })

  it("rejects uppercase REF scheme without throwing", () => {
    expect(
      parseAttachmentRef("REF://attachment/40af0185-316f-42bb-b52b-545d21f6f012")
    ).toBeNull()
  })
})

describe("serializeAttachmentMarkdown", () => {
  it("serializes image as markdown image", () => {
    const attrs: XuanchuAttachmentAttrs = {
      id: "40af0185-316f-42bb-b52b-545d21f6f012",
      label: "架构图",
      image: true,
    }
    expect(serializeAttachmentMarkdown(attrs)).toBe(
      "![架构图](ref://attachment/40af0185-316f-42bb-b52b-545d21f6f012)"
    )
  })

  it("serializes file as markdown link", () => {
    const attrs: XuanchuAttachmentAttrs = {
      id: "a801f977-c745-4f47-95a4-7893a9317aba",
      label: "需求.pdf",
      image: false,
    }
    expect(serializeAttachmentMarkdown(attrs)).toBe(
      "[需求.pdf](ref://attachment/a801f977-c745-4f47-95a4-7893a9317aba)"
    )
  })

  it("strips brackets from label to keep markdown well-formed", () => {
    const attrs: XuanchuAttachmentAttrs = {
      id: "40af0185-316f-42bb-b52b-545d21f6f012",
      label: "weird [name]",
      image: true,
    }
    expect(serializeAttachmentMarkdown(attrs)).toBe(
      "![weird name](ref://attachment/40af0185-316f-42bb-b52b-545d21f6f012)"
    )
  })
})

describe("attachment markdown integration", () => {
  it.each([
    [
      "![架构图](ref://attachment/40af0185-316f-42bb-b52b-545d21f6f012)",
      true,
    ],
    [
      "[需求.pdf](ref://attachment/a801f977-c745-4f47-95a4-7893a9317aba)",
      false,
    ],
  ])("parses and serializes canonical attachment markdown", (source, image) => {
    const document = parseMarkdownToJSON(source)
    const attachment = findNode(document, "xuanchuAttachment")

    expect(attachment).toMatchObject({
      type: "xuanchuAttachment",
      attrs: { image },
    })
    expect(markdownManager.serialize(document).trim()).toBe(source)
  })
})

function findNode(document: { type?: string; content?: unknown[] }, type: string): unknown {
  if (document.type === type) return document
  for (const child of document.content ?? []) {
    if (typeof child === "object" && child !== null) {
      const found = findNode(child as { type?: string; content?: unknown[] }, type)
      if (found) return found
    }
  }
  return undefined
}

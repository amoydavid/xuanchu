import { describe, expect, it } from "vitest"

import {
  parseAttachmentRef,
  serializeAttachmentMarkdown,
  type XuanchuAttachmentAttrs,
} from "./attachment-extension"

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

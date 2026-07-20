import { describe, expect, it } from "vitest"

import { parseReferenceRef, serializeReferenceMarkdown } from "./reference-extension"
import { markdownManager, parseMarkdownToJSON } from "./extensions"

describe("parseReferenceRef", () => {
  it("accepts canonical user ref", () => {
    expect(
      parseReferenceRef("ref://user/8c8b1bed-2e75-4de8-8d5f-c94cbf2b3001")
    ).toEqual({ kind: "user", id: "8c8b1bed-2e75-4de8-8d5f-c94cbf2b3001" })
  })

  it("accepts canonical task ref", () => {
    expect(
      parseReferenceRef("ref://task/61f2a51e-0d5d-4f29-b502-cd195dfa1d84")
    ).toEqual({ kind: "task", id: "61f2a51e-0d5d-4f29-b502-cd195dfa1d84" })
  })

  it("returns null for non-ref scheme", () => {
    expect(parseReferenceRef("https://example.com/a")).toBeNull()
  })

  it("returns null for attachment ref", () => {
    expect(
      parseReferenceRef("ref://attachment/40af0185-316f-42bb-b52b-545d21f6f012")
    ).toBeNull()
  })

  it("returns null for malformed ref", () => {
    expect(parseReferenceRef("ref://user/not-a-uuid")).toBeNull()
    expect(parseReferenceRef("ref://user/8c8b1bed-2e75-4de8-8d5f-c94cbf2b3001/extra")).toBeNull()
  })
})

describe("serializeReferenceMarkdown", () => {
  it("serializes user reference as link", () => {
    expect(
      serializeReferenceMarkdown({
        kind: "user",
        id: "8c8b1bed-2e75-4de8-8d5f-c94cbf2b3001",
        label: "Alice",
      })
    ).toBe("[Alice](ref://user/8c8b1bed-2e75-4de8-8d5f-c94cbf2b3001)")
  })

  it("serializes task reference as link", () => {
    expect(
      serializeReferenceMarkdown({
        kind: "task",
        id: "61f2a51e-0d5d-4f29-b502-cd195dfa1d84",
        label: "#agentapi-17 · 补齐接口",
      })
    ).toBe("[#agentapi-17 · 补齐接口](ref://task/61f2a51e-0d5d-4f29-b502-cd195dfa1d84)")
  })

  it("strips brackets from label", () => {
    expect(
      serializeReferenceMarkdown({
        kind: "user",
        id: "8c8b1bed-2e75-4de8-8d5f-c94cbf2b3001",
        label: "weird [name]",
      })
    ).toBe("[weird name](ref://user/8c8b1bed-2e75-4de8-8d5f-c94cbf2b3001)")
  })
})

describe("reference markdown integration", () => {
  it.each([
    ["[@Alice](ref://user/8c8b1bed-2e75-4de8-8d5f-c94cbf2b3001)", "user"],
    ["[#接口](ref://task/61f2a51e-0d5d-4f29-b502-cd195dfa1d84)", "task"],
  ])("parses and serializes canonical reference markdown", (source, kind) => {
    const document = parseMarkdownToJSON(source)
    const reference = findNode(document, "xuanchuReference")

    expect(reference).toMatchObject({
      type: "xuanchuReference",
      attrs: { kind },
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

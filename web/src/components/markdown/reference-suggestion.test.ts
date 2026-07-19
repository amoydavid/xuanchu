import { afterEach, describe, expect, it } from "vitest"

import {
  buildInsertedReferenceMarkdown,
  detectReferenceTrigger,
  pickSuggestionLabel,
  shouldFetchQuery,
} from "./reference-suggestion"

afterEach(() => {
  // 无共享状态需要清理。
})

describe("detectReferenceTrigger", () => {
  it("triggers on @ at line start", () => {
    expect(detectReferenceTrigger({ textBeforeCaret: "@", composing: false })).toEqual({
      kind: "user",
      query: "",
      startOffset: 0,
    })
  })

  it("triggers on # after space", () => {
    expect(
      detectReferenceTrigger({ textBeforeCaret: "prefix #task", composing: false })
    ).toEqual({
      kind: "task",
      query: "task",
      startOffset: 7,
    })
  })

  it("does not trigger when composing", () => {
    expect(
      detectReferenceTrigger({ textBeforeCaret: "@爱", composing: true })
    ).toBeNull()
  })

  it("does not trigger in a@b.com", () => {
    expect(
      detectReferenceTrigger({ textBeforeCaret: "a@b.com", composing: false })
    ).toBeNull()
  })

  it("does not trigger after whitespace without @ or #", () => {
    expect(
      detectReferenceTrigger({ textBeforeCaret: "hello world", composing: false })
    ).toBeNull()
  })

  it("stops at whitespace after trigger", () => {
    // "@alice " 末尾有空格 → trigger 失效。
    expect(
      detectReferenceTrigger({ textBeforeCaret: "@alice ", composing: false })
    ).toBeNull()
  })

  it("supports unicode query", () => {
    expect(
      detectReferenceTrigger({ textBeforeCaret: "你好 @爱丽丝", composing: false })
    ).toEqual({
      kind: "user",
      query: "爱丽丝",
      startOffset: 3,
    })
  })
})

describe("shouldFetchQuery", () => {
  it("fetches when at least 1 non-whitespace char", () => {
    expect(shouldFetchQuery("a")).toBe(true)
    expect(shouldFetchQuery("alice")).toBe(true)
  })

  it("does not fetch empty or whitespace", () => {
    expect(shouldFetchQuery("")).toBe(false)
    expect(shouldFetchQuery(" ")).toBe(false)
    expect(shouldFetchQuery("\t")).toBe(false)
  })
})

describe("buildInsertedReferenceMarkdown", () => {
  it("builds user mention markdown", () => {
    expect(
      buildInsertedReferenceMarkdown({
        kind: "user",
        id: "8c8b1bed-2e75-4de8-8d5f-c94cbf2b3001",
        label: "Alice",
      })
    ).toBe("[Alice](ref://user/8c8b1bed-2e75-4de8-8d5f-c94cbf2b3001)")
  })

  it("builds task reference markdown with slug prefix", () => {
    expect(
      buildInsertedReferenceMarkdown({
        kind: "task",
        id: "61f2a51e-0d5d-4f29-b502-cd195dfa1d84",
        label: "#agentapi-17 · 补齐接口",
      })
    ).toBe("[#agentapi-17 · 补齐接口](ref://task/61f2a51e-0d5d-4f29-b502-cd195dfa1d84)")
  })

  it("strips brackets from label", () => {
    expect(
      buildInsertedReferenceMarkdown({
        kind: "user",
        id: "8c8b1bed-2e75-4de8-8d5f-c94cbf2b3001",
        label: "weird [x]",
      })
    ).toBe("[weird x](ref://user/8c8b1bed-2e75-4de8-8d5f-c94cbf2b3001)")
  })
})

describe("pickSuggestionLabel", () => {
  it("prefers display_name for user", () => {
    expect(
      pickSuggestionLabel({
        type: "user",
        id: "u1",
        status: "resolved",
        user: { id: "u1", name: "alice", display_name: "Alice Lee" },
      })
    ).toBe("Alice Lee")
  })

  it("falls back to name for user", () => {
    expect(
      pickSuggestionLabel({
        type: "user",
        id: "u1",
        status: "resolved",
        user: { id: "u1", name: "alice" },
      })
    ).toBe("alice")
  })

  it("builds task slug label", () => {
    expect(
      pickSuggestionLabel({
        type: "task",
        id: "t1",
        status: "resolved",
        task: { id: "t1", title: "Implement", task_slug: "agentapi-17" },
      })
    ).toBe("#agentapi-17 · Implement")
  })

  it("falls back to title when no slug", () => {
    expect(
      pickSuggestionLabel({
        type: "task",
        id: "t1",
        status: "resolved",
        task: { id: "t1", title: "Implement" },
      })
    ).toBe("Implement")
  })

  it("returns empty for unavailable", () => {
    expect(
      pickSuggestionLabel({ type: "user", id: "x", status: "unavailable" })
    ).toBe("")
  })
})

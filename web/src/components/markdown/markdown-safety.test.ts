import { describe, expect, it } from "vitest"

import { parseMarkdownToJSON } from "./extensions"
import { escapeMarkdownHtml, isAllowedMarkdownHref } from "./markdown-safety"

describe("markdown safety", () => {
  it("escapes raw html tags outside code", () => {
    expect(escapeMarkdownHtml("<script>alert(1)</script>")).toBe(
      "&lt;script&gt;alert(1)&lt;/script&gt;"
    )
    expect(escapeMarkdownHtml('<img src=x onerror="alert(1)">')).toBe(
      '&lt;img src=x onerror="alert(1)"&gt;'
    )
  })

  it("keeps inline code and fenced code untouched", () => {
    expect(escapeMarkdownHtml("`<script>`")).toBe("`<script>`")
    expect(escapeMarkdownHtml("```html\n<script>\n```")).toBe(
      "```html\n<script>\n```"
    )
  })

  it("does not rewrite normal markdown links", () => {
    expect(escapeMarkdownHtml("[官网](https://example.com?a=<b>)")).toBe(
      "[官网](https://example.com?a=<b>)"
    )
  })

  it("allows only explicit safe protocols", () => {
    expect(isAllowedMarkdownHref("https://example.com")).toBe(true)
    expect(isAllowedMarkdownHref("http://example.com")).toBe(true)
    expect(isAllowedMarkdownHref("mailto:ops@example.com")).toBe(true)
    expect(isAllowedMarkdownHref("javascript:alert(1)")).toBe(false)
    expect(isAllowedMarkdownHref("data:text/html,evil")).toBe(false)
    expect(isAllowedMarkdownHref("/relative")).toBe(false)
  })

  it("parses markdown through the shared manager", () => {
    const json = parseMarkdownToJSON("# 标题\n\n- [x] 完成")

    expect(json.type).toBe("doc")
    expect(JSON.stringify(json)).toContain("标题")
  })
})

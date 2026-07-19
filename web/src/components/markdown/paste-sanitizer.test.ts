import { describe, expect, it } from "vitest"

import { sanitizeRichPaste } from "./paste-sanitizer"

describe("sanitizeRichPaste", () => {
  it("keeps markdown-schema html and removes active content", () => {
    const result = sanitizeRichPaste({
      html: `<h1 style="color:red">标题</h1><script>x()</script><table><tr><td onclick="x()">A</td></tr></table>`,
      files: [],
    })
    expect(result.html).toContain("<h1>标题</h1>")
    expect(result.html).toContain("<table>")
    expect(result.html).not.toMatch(/style|script|onclick/)
  })

  it("replaces remote images with controlled markers", () => {
    const result = sanitizeRichPaste({
      html: `<p>A<img src="https://cdn.example.com/x.png" alt="X">B</p>`,
      files: [],
    })
    expect(result.images).toHaveLength(1)
    expect(result.images[0]?.kind).toBe("remote")
    expect(result.images[0]?.sourceURL).toBe("https://cdn.example.com/x.png")
    expect(result.html).toContain(
      `data-xuanchu-paste-image="${result.images[0]?.key}"`
    )
    expect(result.html).not.toContain("<img")
  })

  it("prefers clipboard files over remote src", () => {
    const file = new File([new Uint8Array([1])], "screenshot.png", {
      type: "image/png",
    })
    const result = sanitizeRichPaste({
      html: `<p>A<img src="https://cdn.example.com/x.png" alt="X">B</p>`,
      files: [file],
    })
    expect(result.images[0]?.kind).toBe("file")
    expect(result.images[0]?.file).toBe(file)
  })

  it("accepts data url images as candidates", () => {
    const result = sanitizeRichPaste({
      html: `<img src="data:image/png;base64,xx" alt="Y">`,
      files: [],
    })
    expect(result.images[0]?.kind).toBe("data")
    expect(result.images[0]?.sourceURL).toBe("data:image/png;base64,xx")
  })

  it("rejects javascript: URLs entirely", () => {
    const result = sanitizeRichPaste({
      html: `<a href="javascript:alert(1)">click</a>`,
      files: [],
    })
    expect(result.html).not.toContain("javascript:")
  })

  it("picks highest-resolution srcset entry", () => {
    const result = sanitizeRichPaste({
      html: `<img srcset="https://cdn.example.com/x.png 100w, https://cdn.example.com/x@2x.png 500w" alt="Z">`,
      files: [],
    })
    expect(result.images[0]?.sourceURL).toBe("https://cdn.example.com/x@2x.png")
  })

  it("normalizes remote url case and strips fragment", () => {
    const result = sanitizeRichPaste({
      html: `<img src="HTTPS://Example.com/a.png?token=1#frag" alt="N">`,
      files: [],
    })
    expect(result.images[0]?.sourceURL).toBe("https://example.com/a.png?token=1")
  })
})

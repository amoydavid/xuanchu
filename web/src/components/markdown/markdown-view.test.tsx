import { render, screen } from "@testing-library/react"
import { describe, expect, it } from "vitest"

import { MarkdownView } from "./markdown-view"

describe("MarkdownView", () => {
  it("renders common markdown nodes", () => {
    render(
      <MarkdownView>{`# 标题

- 列表

> 引用

\`\`\`ts
const a = 1
\`\`\``}</MarkdownView>
    )

    expect(screen.getByRole("heading", { name: "标题" })).toBeTruthy()
    expect(screen.getByText("列表")).toBeTruthy()
    expect(screen.getByText("引用")).toBeTruthy()
    expect(screen.getByText("const a = 1")).toBeTruthy()
  })

  it("renders raw html as text", () => {
    const { container } = render(
      <MarkdownView>{"<script>alert(1)</script>"}</MarkdownView>
    )

    expect(container.querySelector("script")).toBeNull()
    expect(screen.getByText("<script>alert(1)</script>")).toBeTruthy()
  })

  it("does not render javascript links as clickable hrefs", () => {
    const { container } = render(
      <MarkdownView>
        {"[bad](javascript:alert(1)) [good](https://example.com)"}
      </MarkdownView>
    )

    expect(container.querySelector('a[href^="javascript:"]')).toBeNull()
    expect(container.querySelector('a[href="https://example.com"]')).toBeTruthy()
  })
})

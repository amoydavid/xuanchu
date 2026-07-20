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

  it("offsets heading levels when embedded below a page title", () => {
    render(
      <MarkdownView headingOffset={1}>{`# 一级

## 二级

###### 六级`}</MarkdownView>
    )

    expect(screen.getByRole("heading", { level: 2, name: "一级" })).toBeTruthy()
    expect(screen.getByRole("heading", { level: 3, name: "二级" })).toBeTruthy()
    expect(screen.getByRole("heading", { level: 6, name: "六级" })).toBeTruthy()
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

  it("falls back to the saved attachment label without emitting a broken image when no task context exists", () => {
    const { container } = render(
      <MarkdownView>{"![架构图](ref://attachment/40af0185-316f-42bb-b52b-545d21f6f012)"}</MarkdownView>
    )

    expect(screen.getByText("架构图")).toBeTruthy()
    expect(container.querySelector("img")).toBeNull()
  })
})

import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { useState, type ReactNode } from "react"
import { describe, expect, it, vi } from "vitest"

import { TooltipProvider } from "@/components/ui/tooltip"

import { MarkdownEditor } from "./markdown-editor"

// 编辑器工具栏使用 Radix Tooltip，测试中需要 Provider 上下文
function renderWithTooltip(ui: ReactNode) {
  return render(<TooltipProvider>{ui}</TooltipProvider>)
}

describe("MarkdownEditor", () => {
  it("renders initial markdown content", async () => {
    renderWithTooltip(
      <MarkdownEditor onChange={() => undefined} value="# 标题" />
    )

    expect(await screen.findByText("标题")).toBeTruthy()
  })

  it("calls onModEnter for keyboard save", async () => {
    const onModEnter = vi.fn()
    renderWithTooltip(
      <MarkdownEditor
        ariaLabel="任务描述"
        onChange={() => undefined}
        onModEnter={onModEnter}
        value="内容"
      />
    )

    await userEvent.click(screen.getByLabelText("任务描述"))
    await userEvent.keyboard("{Control>}{Enter}{/Control}")

    expect(onModEnter).toHaveBeenCalled()
  })

  it("reports sanitized markdown changes", async () => {
    const onChange = vi.fn()
    renderWithTooltip(
      <MarkdownEditor ariaLabel="任务描述" onChange={onChange} value="" />
    )

    await userEvent.click(screen.getByLabelText("任务描述"))
    await userEvent.keyboard("<script>alert(1)</script>")

    await waitFor(() => {
      expect(onChange).toHaveBeenCalled()
    })
    expect(onChange.mock.calls.at(-1)?.[0]).not.toContain("<script>")
  })

  it("切换源码模式后可直接编辑原始 markdown", async () => {
    const onChange = vi.fn()
    function ControlledEditor() {
      const [value, setValue] = useState("# 旧标题")
      return (
        <MarkdownEditor
          ariaLabel="任务描述"
          onChange={(v) => {
            setValue(v)
            onChange(v)
          }}
          value={value}
        />
      )
    }
    renderWithTooltip(<ControlledEditor />)

    // 切换到源码
    await userEvent.click(screen.getByLabelText("切换到源码"))

    // 源码模式下出现 textarea，可见原始 markdown
    const sourceArea = await screen.findByLabelText("源码编辑器")
    expect(sourceArea.tagName).toBe("TEXTAREA")
    expect((sourceArea as HTMLTextAreaElement).value).toContain("# 旧标题")

    // 在源码模式直接编辑，触发 onChange
    await userEvent.clear(sourceArea)
    await userEvent.type(sourceArea, "# 新标题")

    expect(onChange).toHaveBeenCalled()
    // type 逐字符触发 onChange，检查最后一次（完整值）
    expect(onChange.mock.calls.at(-1)?.[0]).toContain("新标题")
  })

  it("源码模式编辑后切回富文本能渲染新内容", async () => {
    function ControlledEditor() {
      const [value, setValue] = useState("")
      return (
        <MarkdownEditor
          ariaLabel="任务描述"
          onChange={setValue}
          value={value}
        />
      )
    }

    renderWithTooltip(<ControlledEditor />)

    await userEvent.click(screen.getByLabelText("切换到源码"))
    const sourceArea = await screen.findByLabelText("源码编辑器")
    await userEvent.type(sourceArea, "# 渲染标题")
    await userEvent.click(screen.getByLabelText("切换到富文本"))

    expect(await screen.findByText("渲染标题")).toBeTruthy()
  })

  it("点击链接按钮打开 Dialog 并校验非法输入", async () => {
    renderWithTooltip(
      <MarkdownEditor ariaLabel="任务描述" onChange={() => undefined} value="" />
    )

    await userEvent.click(screen.getByLabelText("链接"))

    // Dialog 出现
    expect(await screen.findByText("链接地址")).toBeTruthy()
    const input = screen.getByLabelText("链接地址")

    // 输入非法协议，提交后显示错误
    await userEvent.clear(input)
    await userEvent.type(input, "ftp://bad")
    await userEvent.click(screen.getByRole("button", { name: "确定" }))

    expect(
      await screen.findByText("仅支持 http:// / https:// / mailto: 链接")
    ).toBeTruthy()
  })

  it("表格浮动菜单按选区状态切换按钮", async () => {
    renderWithTooltip(
      <MarkdownEditor ariaLabel="任务描述" onChange={() => undefined} value="" />
    )

    await userEvent.click(screen.getByLabelText("任务描述"))
    await userEvent.click(screen.getByLabelText("插入表格"))

    // 默认单元格选区：出现「选中整行/整列」入口 + 插入按钮，不出现删除按钮
    expect(await screen.findByLabelText("选中整行")).toBeTruthy()
    expect(screen.getByLabelText("选中整列")).toBeTruthy()
    expect(screen.getByLabelText("上方加行")).toBeTruthy()
    expect(screen.queryByLabelText("删除行")).toBeNull()

    // 点击「选中整行」后，菜单切换为整行操作：出现「删除行」
    await userEvent.click(screen.getByLabelText("选中整行"))
    expect(await screen.findByLabelText("删除行")).toBeTruthy()
    expect(screen.queryByLabelText("选中整行")).toBeNull()

    // 整行状态下仍能加行
    expect(screen.getByLabelText("上方加行")).toBeTruthy()
  })
})

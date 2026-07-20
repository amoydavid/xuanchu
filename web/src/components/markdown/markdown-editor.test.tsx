import { fireEvent, render, screen, waitFor } from "@testing-library/react"
import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import userEvent from "@testing-library/user-event"
import { useState, type ReactNode } from "react"
import { afterEach, describe, expect, it, vi } from "vitest"

import { TooltipProvider } from "@/components/ui/tooltip"

import { MarkdownEditor } from "./markdown-editor"
import type { AttachmentUploadQueueAPI } from "./attachment-upload-queue"

const pastedAttachment = {
  id: "40af0185-316f-42bb-b52b-545d21f6f012",
  attached_to: { type: "task" as const, id: "task-1" },
  state: "draft" as const,
  original_name: "截图.png",
  display_name: "截图.png",
  media_type: "image/png",
  extension: "png",
  size_bytes: 3,
  sha256: "abc",
  inline_capable: true,
  source_type: "upload" as const,
  content_url: "/attachment/content",
  created_by: { type: "user" },
  created_at: 0,
  modified_at: 0,
}

// 编辑器工具栏使用 Radix Tooltip，测试中需要 Provider 上下文
function renderWithTooltip(ui: ReactNode) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(<QueryClientProvider client={queryClient}><TooltipProvider>{ui}</TooltipProvider></QueryClientProvider>)
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe("MarkdownEditor", () => {
  it("uploads an image clipboard File and emits canonical attachment markdown", async () => {
    const uploadFile = vi.fn().mockResolvedValue(pastedAttachment)
    const api: AttachmentUploadQueueAPI = {
      uploadFile,
      importRemoteURL: vi.fn(),
      removeDraft: vi.fn(),
    }
    const onChange = vi.fn()
    renderWithTooltip(
      <MarkdownEditor
        ariaLabel="任务描述"
        attachmentContext={{
          workspaceSlug: "workspace-1",
          taskRef: "task-1",
          attachmentAPI: api,
          fetchSuggestions: vi.fn(),
        }}
        onChange={onChange}
        value=""
      />
    )

    const image = new File(["png"], "截图.png", { type: "image/png" })
    fireEvent.paste(screen.getByRole("textbox", { name: "任务描述" }), {
      clipboardData: {
        items: [
          {
            kind: "file",
            getAsFile: () => image,
          },
        ],
        getData: () => "",
      },
    })

    await waitFor(() => expect(uploadFile).toHaveBeenCalledWith(
      "task-1",
      expect.objectContaining({ file: image, kind: "file" }),
      expect.any(AbortSignal)
    ))
    await waitFor(() =>
      expect(onChange).toHaveBeenLastCalledWith(
        "![截图.png](ref://attachment/40af0185-316f-42bb-b52b-545d21f6f012)"
      )
    )
  })

  it("renders a local preview instead of exposing the deferred marker while creating a task", async () => {
    const onDeferredAttachment = vi.fn()
    const onChange = vi.fn()
    const createObjectURL = vi.fn(() => "blob:deferred-preview")
    vi.stubGlobal("URL", {
      createObjectURL,
      revokeObjectURL: vi.fn(),
    })
    renderWithTooltip(
      <MarkdownEditor
        ariaLabel="新建任务内容"
        attachmentContext={{
          workspaceSlug: "workspace-1",
          taskRef: "",
          fetchSuggestions: vi.fn(),
        }}
        onDeferredAttachment={onDeferredAttachment}
        onChange={onChange}
        value=""
      />
    )

    const image = new File(["png"], "截图.png", { type: "image/png" })
    fireEvent.paste(screen.getByRole("textbox", { name: "新建任务内容" }), {
      clipboardData: {
        items: [{ kind: "file", getAsFile: () => image }],
        getData: () => "",
      },
    })

    expect((await screen.findByRole("img", { name: "截图.png" })).getAttribute("src")).toBe("blob:deferred-preview")
    expect(screen.getByRole("textbox", { name: "新建任务内容" }).textContent).not.toContain(
      "[[xuanchu-paste:"
    )
    expect(onDeferredAttachment).toHaveBeenCalledWith(
      expect.objectContaining({ candidate: expect.objectContaining({ file: image }) })
    )
    expect(onChange.mock.calls.at(-1)?.[0]).toContain("[[xuanchu-paste:")
  })

  it("transfers a remote rich-text image to the server instead of persisting its URL", async () => {
    const importRemoteURL = vi.fn().mockResolvedValue(pastedAttachment)
    const onChange = vi.fn()
    renderWithTooltip(
      <MarkdownEditor
        ariaLabel="任务描述"
        attachmentContext={{
          workspaceSlug: "workspace-1", taskRef: "task-1", fetchSuggestions: vi.fn(),
          attachmentAPI: { uploadFile: vi.fn(), importRemoteURL, removeDraft: vi.fn() },
        }}
        onChange={onChange}
        value=""
      />
    )

    fireEvent.paste(screen.getByRole("textbox", { name: "任务描述" }), {
      clipboardData: { items: [], getData: (type: string) => type === "text/html" ? '<p>前<img src="https://cdn.example.test/diagram.png" alt="架构图">后</p>' : "" },
    })

    await waitFor(() => expect(importRemoteURL).toHaveBeenCalledWith("task-1", "https://cdn.example.test/diagram.png", expect.any(AbortSignal)))
    await waitFor(() => expect(onChange.mock.calls.at(-1)?.[0]).toContain("ref://attachment/40af0185-316f-42bb-b52b-545d21f6f012"))
    expect(onChange.mock.calls.at(-1)?.[0]).not.toContain("cdn.example.test")
  })

  it("allows a failed HTTP remote image to remain as an explicit ordinary link", async () => {
    const onChange = vi.fn()
    renderWithTooltip(
      <MarkdownEditor
        ariaLabel="任务描述"
        attachmentContext={{
          workspaceSlug: "workspace-1", taskRef: "task-1", fetchSuggestions: vi.fn(),
          attachmentAPI: { uploadFile: vi.fn(), importRemoteURL: vi.fn().mockRejectedValue(new Error("fetch failed")), removeDraft: vi.fn() },
        }}
        onChange={onChange}
        value=""
      />
    )

    fireEvent.paste(screen.getByRole("textbox", { name: "任务描述" }), {
      clipboardData: { items: [], getData: (type: string) => type === "text/html" ? '<img src="http://cdn.example.test/diagram.png" alt="架构图">' : "" },
    })

    await userEvent.click(await screen.findByRole("button", { name: "保留链接" }))
    await waitFor(() => expect(onChange.mock.calls.at(-1)?.[0]).toContain("[架构图](http://cdn.example.test/diagram.png)"))
  })

  it("uploads a data image instead of persisting a data URL", async () => {
    const uploadFile = vi.fn().mockResolvedValue(pastedAttachment)
    const onChange = vi.fn()
    renderWithTooltip(
      <MarkdownEditor
        ariaLabel="任务描述"
        attachmentContext={{
          workspaceSlug: "workspace-1", taskRef: "task-1", fetchSuggestions: vi.fn(),
          attachmentAPI: { uploadFile, importRemoteURL: vi.fn(), removeDraft: vi.fn() },
        }}
        onChange={onChange}
        value=""
      />
    )

    fireEvent.paste(screen.getByRole("textbox", { name: "任务描述" }), {
      clipboardData: { items: [], getData: (type: string) => type === "text/html" ? '<img src="data:image/png;base64,cG5n" alt="内嵌图">' : "" },
    })

    await waitFor(() => expect(uploadFile).toHaveBeenCalledWith("task-1", expect.objectContaining({ file: expect.objectContaining({ type: "image/png" }) }), expect.any(AbortSignal)))
    await waitFor(() => expect(onChange.mock.calls.at(-1)?.[0]).toContain("ref://attachment/40af0185-316f-42bb-b52b-545d21f6f012"))
    expect(onChange.mock.calls.at(-1)?.[0]).not.toContain("data:image")
  })

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

  it("Tab 聚焦时进入编辑区而非工具栏按钮", async () => {
    renderWithTooltip(
      <>
        <input data-testid="before" />
        <MarkdownEditor ariaLabel="任务描述" onChange={() => undefined} value="" />
      </>
    )

    // 从外部可聚焦元素 Tab 进入组件
    await userEvent.click(screen.getByTestId("before"))
    await userEvent.tab()

    // 焦点应落在编辑区，而不是工具栏的第一个按钮（加粗）
    expect(document.activeElement).toBe(
      screen.getByRole("textbox", { name: "任务描述" })
    )
    expect(document.activeElement).not.toBe(screen.getByLabelText("加粗"))
  })

  it("源码模式下 Tab 聚焦时进入 textarea 而非工具栏按钮", async () => {
    renderWithTooltip(
      <>
        <input data-testid="before" />
        <MarkdownEditor ariaLabel="任务描述" onChange={() => undefined} value="" />
      </>
    )

    await userEvent.click(screen.getByLabelText("切换到源码"))
    await userEvent.click(screen.getByTestId("before"))
    await userEvent.tab()

    expect(document.activeElement).toBe(
      screen.getByLabelText("源码编辑器")
    )
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

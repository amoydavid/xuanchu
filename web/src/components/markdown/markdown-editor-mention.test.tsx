import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { useState, type ReactNode } from "react"
import { describe, expect, it, vi } from "vitest"

import { TooltipProvider } from "@/components/ui/tooltip"

import { MarkdownEditor } from "./markdown-editor"
import type { ReferenceSuggestionMenuItem } from "./reference-suggestion-menu"

// 集成测试：覆盖 @ mention 的完整链路：
// - @tiptap/suggestion 引擎触发检测
// - fetchSuggestions 调用（含 debounce）
// - 候选渲染 + 键盘选中
// - 选中后插入 xuanchuReference 节点（ref://user/{id} 序列化）

function makeItem(id: string, label: string): ReferenceSuggestionMenuItem {
  return {
    id,
    kind: "user",
    label,
    description: id,
    resolution: {
      type: "user",
      id,
      status: "resolved",
      user: { id, name: id, display_name: label },
    },
  }
}

function renderWithTooltip(ui: ReactNode) {
  return render(<TooltipProvider>{ui}</TooltipProvider>)
}

// ControlledEditor 把 onChange 暴露出来，便于断言插入后的 markdown。
function ControlledEditor({
  fetchSuggestions,
  onValue,
}: {
  fetchSuggestions: (input: {
    kind: "user" | "task"
    query: string
    signal: AbortSignal
  }) => Promise<ReferenceSuggestionMenuItem[]>
  onValue?: (value: string) => void
}) {
  const [value, setValue] = useState("")
  return (
    <MarkdownEditor
      ariaLabel="任务描述"
      attachmentContext={{
        workspaceSlug: "ws",
        taskRef: "task-1",
        fetchSuggestions,
      }}
      onChange={(next) => {
        setValue(next)
        onValue?.(next)
      }}
      value={value}
    />
  )
}

describe("MarkdownEditor mention (@)", () => {
  it("输入 @a 后弹出候选菜单并选中插入 ref://user 引用", async () => {
    const user = userEvent.setup()
    const fetch = vi.fn(async () => [makeItem("u1", "Alice"), makeItem("u2", "Bob")])
    const values: string[] = []
    renderWithTooltip(
      <ControlledEditor fetchSuggestions={fetch} onValue={(v) => values.push(v)} />
    )

    await user.click(screen.getByRole("textbox", { name: "任务描述" }))
    await user.keyboard("@a")

    // listbox 出现，且候选项渲染。
    const alice = await screen.findByRole("option", { name: /Alice/ }, { timeout: 3000 })
    expect(alice).toBeTruthy()

    // 点击选中。
    await user.click(alice)

    // onChange 最终应包含 ref://user/u1 的 markdown 链接（xuanchuReference 序列化）。
    await waitFor(() => {
      expect(values.some((v) => v.includes("ref://user/u1"))).toBe(true)
    })
  })

  it("方向键 + Enter 选中第二个候选", async () => {
    const user = userEvent.setup()
    const fetch = vi.fn(async () => [makeItem("u1", "Alice"), makeItem("u2", "Bob")])
    renderWithTooltip(<ControlledEditor fetchSuggestions={fetch} />)

    await user.click(screen.getByRole("textbox", { name: "任务描述" }))
    await user.keyboard("@a")

    await screen.findByRole("option", { name: /Alice/ }, { timeout: 3000 })

    // ArrowDown 选中 Bob，Enter 确认。
    await user.keyboard("{ArrowDown}{Enter}")

    // 选中后菜单关闭（listbox 消失）。
    await waitFor(() => {
      expect(screen.queryByRole("option", { name: /Bob/ })).toBeNull()
    })
  })

  it("仅输入 @（空 query）时也应列出成员，而非显示无结果", async () => {
    const user = userEvent.setup()
    const fetch = vi.fn(async () => [makeItem("u1", "Alice"), makeItem("u2", "Bob")])
    renderWithTooltip(<ControlledEditor fetchSuggestions={fetch} />)

    await user.click(screen.getByRole("textbox", { name: "任务描述" }))
    await user.keyboard("@")

    // 空 query 仍会请求并渲染成员列表。
    const alice = await screen.findByRole("option", { name: /Alice/ }, { timeout: 3000 })
    expect(alice).toBeTruthy()
    expect(screen.getByRole("option", { name: /Bob/ })).toBeTruthy()
    // 确认确实发起了请求（query 为空字符串）。
    expect(fetch).toHaveBeenCalled()
    expect(fetch.mock.calls.at(-1)?.[0].query).toBe("")
  })

  it("Escape 关闭候选菜单", async () => {
    const user = userEvent.setup()
    const fetch = vi.fn(async () => [makeItem("u1", "Alice")])
    renderWithTooltip(<ControlledEditor fetchSuggestions={fetch} />)

    await user.click(screen.getByRole("textbox", { name: "任务描述" }))
    await user.keyboard("@a")

    await screen.findByRole("option", { name: /Alice/ }, { timeout: 3000 })

    await user.keyboard("{Escape}")

    await waitFor(() => {
      expect(screen.queryByRole("listbox")).toBeNull()
    })
  })

  it("候选列表在 Dialog 滚动锁下仍可滚动", async () => {
    const user = userEvent.setup()
    // react-remove-scroll 在 document 的 bubble 阶段取消菜单外部滚动。
    // 这里以同样的方式模拟，验证菜单会在到达该监听器前截断原生事件。
    const documentWheelHits: WheelEvent[] = []
    const removeScrollHandler = (event: WheelEvent) => {
      if (event.target instanceof HTMLElement && event.target.closest('[role="listbox"]')) {
        documentWheelHits.push(event)
        event.preventDefault()
      }
    }
    document.addEventListener("wheel", removeScrollHandler)

    // 候选数量超过 maxItems(20) 才会真正溢出可滚动。
    const many = Array.from({ length: 30 }, (_, i) =>
      makeItem(`u${i}`, `User ${i}`)
    )
    const fetch = vi.fn(async () => many)
    try {
      renderWithTooltip(<ControlledEditor fetchSuggestions={fetch} />)

      await user.click(screen.getByRole("textbox", { name: "任务描述" }))
      await user.keyboard("@a")

      const listbox = await screen.findByRole("listbox", {}, { timeout: 3000 })
      expect(
        listbox.parentElement?.classList.contains("pointer-events-auto")
      ).toBe(true)
      // jsdom 不计算布局，手动模拟真实页面中菜单内容溢出的条件。
      Object.defineProperty(listbox, "scrollHeight", { value: 300, configurable: true })
      Object.defineProperty(listbox, "clientHeight", { value: 100, configurable: true })
      listbox.dispatchEvent(
        new WheelEvent("wheel", { bubbles: true, cancelable: true, deltaY: 100 })
      )
      expect(documentWheelHits).toHaveLength(0)
    } finally {
      document.removeEventListener("wheel", removeScrollHandler)
    }
  })
})

import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { ThemeProvider } from "@/components/theme-provider"
import { TooltipProvider } from "@/components/ui/tooltip"
import { i18n } from "@/i18n"

import { AuditConsole } from "./audit-console"

function renderConsole() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={client}>
      <ThemeProvider>
        <TooltipProvider>
          <AuditConsole workspaceSlug="dajee" />
        </TooltipProvider>
      </ThemeProvider>
    </QueryClientProvider>
  )
}

const sampleRows = [
  {
    id: 1,
    action: "task.modify",
    target_type: "task",
    target_id: "TASK-1",
    actor: { id: "u1", name: "alice", display_name: "Alice" },
    created_at: 1756000000,
  },
  {
    id: 2,
    action: "project.transition",
    target_type: "project",
    target_id: "proj-a",
    actor: { id: "u2", name: "bob" },
    created_at: 1756100000,
  },
]

describe("AuditConsole", () => {
  beforeEach(async () => {
    vi.spyOn(globalThis, "fetch").mockImplementation(() =>
      Promise.resolve(
        new Response(JSON.stringify({ data: sampleRows }), { status: 200 })
      )
    )
    await i18n.changeLanguage("zh-CN")
  })

  it("loads and renders audit rows", async () => {
    renderConsole()
    await waitFor(() => {
      expect(screen.getByText("task.modify")).toBeTruthy()
    })
    expect(screen.getByText("project.transition")).toBeTruthy()
  })

  it("shows current-result filter hint, not full-search wording", async () => {
    renderConsole()
    await waitFor(() => {
      expect(screen.getByText("task.modify")).toBeTruthy()
    })
    // 文案必须明确「筛选当前结果」，不能误导为全量 actor/action/time 服务端搜索。
    expect(screen.getByText(/筛选当前结果/)).toBeTruthy()
  })

  it("exports CSV via button", async () => {
    const createObjectURL = vi.fn(() => "blob:fake")
    const clickAnchor = vi.fn()
    Object.defineProperty(globalThis, "URL", {
      value: { ...globalThis.URL, createObjectURL, revokeObjectURL: vi.fn() },
      configurable: true,
    })
    // 拦截 anchor click：jsdom 不会真正下载
    const originalCreateElement = document.createElement.bind(document)
    vi.spyOn(document, "createElement").mockImplementation((tag: string) => {
      const el = originalCreateElement(tag)
      if (tag === "a") {
        el.click = clickAnchor
      }
      return el
    })

    renderConsole()
    await waitFor(() => {
      expect(screen.getByText("task.modify")).toBeTruthy()
    })
    const btn = screen.getByRole("button", { name: /导出/ })
    await userEvent.click(btn)

    expect(createObjectURL).toHaveBeenCalledTimes(1)
    expect(clickAnchor).toHaveBeenCalledTimes(1)
  })

  it("filters current rows by actor input", async () => {
    renderConsole()
    await waitFor(() => {
      expect(screen.getByText("task.modify")).toBeTruthy()
    })
    const input = screen.getByPlaceholderText(/搜索当前结果/)
    await userEvent.type(input, "alice")
    // bob 那行被过滤掉
    expect(screen.queryByText("project.transition")).toBeNull()
    expect(screen.getByText("task.modify")).toBeTruthy()
  })
})

import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { ThemeProvider } from "@/components/theme-provider"
import { TooltipProvider } from "@/components/ui/tooltip"
import { i18n } from "@/i18n"

import { SinkList } from "./sink-list"

function renderList(canWrite = true) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={client}>
      <ThemeProvider>
        <TooltipProvider>
          <SinkList canWrite={canWrite} />
        </TooltipProvider>
      </ThemeProvider>
    </QueryClientProvider>
  )
}

const sinksResponse = {
  data: [
    {
      id: "s1",
      workspace_id: "ws1",
      name: "feishu-bot",
      type: "http_template",
      endpoint_mode: "config_value",
      config_key: "feishu.webhook",
      allowed_hosts: ["open.feishu.cn"],
      enabled: true,
      timeout_seconds: 10,
      max_attempts: 3,
      max_concurrency: 0,
      created_at: 1756000000,
      modified_at: 1756000000,
    },
    {
      id: "s2",
      workspace_id: "ws1",
      name: "audit-stream",
      type: "webhook",
      endpoint_mode: "static_url",
      url: "https://audit.example.com/hook",
      enabled: false,
      timeout_seconds: 10,
      max_attempts: 3,
      max_concurrency: 0,
      created_at: 1756100000,
      modified_at: 1756100000,
    },
  ],
}

function mockFetch() {
  return vi.spyOn(globalThis, "fetch").mockImplementation((input: unknown) => {
    const url = typeof input === "string" ? input : (input as Request).url
    if (url.startsWith("/api/v1/notification-sinks")) {
      return Promise.resolve(new Response(JSON.stringify(sinksResponse), { status: 200 }))
    }
    return Promise.resolve(new Response(JSON.stringify({ data: {} }), { status: 200 }))
  })
}

describe("SinkList", () => {
  beforeEach(async () => {
    mockFetch()
    await i18n.changeLanguage("zh-CN")
  })

  it("lists sinks with name, type, endpoint and status", async () => {
    renderList()
    await waitFor(() => {
      expect(screen.getByText("feishu-bot")).toBeTruthy()
    })
    expect(screen.getByText("audit-stream")).toBeTruthy()
    expect(screen.getByText("https://audit.example.com/hook")).toBeTruthy()
  })

  it("shows write actions for write users", async () => {
    renderList(true)
    await waitFor(() => {
      expect(screen.getByText("feishu-bot")).toBeTruthy()
    })
    const editButtons = screen.getAllByRole("button", { name: /编辑 Sink/ })
    expect(editButtons.length).toBeGreaterThan(0)
  })

  it("hides write actions for read-only users", async () => {
    renderList(false)
    await waitFor(() => {
      expect(screen.getByText("feishu-bot")).toBeTruthy()
    })
    expect(screen.queryByRole("button", { name: /新建 Sink/ })).toBeNull()
    expect(screen.queryByRole("button", { name: /编辑 Sink/ })).toBeNull()
  })

  it("shows empty state when no sinks", async () => {
    vi.spyOn(globalThis, "fetch").mockImplementation(() =>
      Promise.resolve(new Response(JSON.stringify({ data: [] }), { status: 200 }))
    )
    renderList()
    await waitFor(() => {
      expect(screen.getByText(/还没有 Sink/)).toBeTruthy()
    })
  })

  it("opens create dialog on new sink click", async () => {
    renderList(true)
    await waitFor(() => {
      expect(screen.getByText("feishu-bot")).toBeTruthy()
    })
    await userEvent.click(screen.getByRole("button", { name: /新建 Sink/ }))
    // Dialog 内容在 portal 中渲染；用对话框标题（与按钮文案一致）的多个匹配作为打开信号。
    await waitFor(() => {
      const matches = screen.getAllByText(/新建 Sink/)
      expect(matches.length).toBeGreaterThanOrEqual(2)
    })
  })
})

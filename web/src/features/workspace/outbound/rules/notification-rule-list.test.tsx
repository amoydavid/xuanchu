import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { fireEvent, render, screen, waitFor } from "@testing-library/react"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { ThemeProvider } from "@/components/theme-provider"
import { TooltipProvider } from "@/components/ui/tooltip"
import { i18n } from "@/i18n"

import { NotificationRuleList } from "./notification-rule-list"

function renderList(canWrite = true) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={client}>
      <ThemeProvider>
        <TooltipProvider>
          <NotificationRuleList canWrite={canWrite} />
        </TooltipProvider>
      </ThemeProvider>
    </QueryClientProvider>
  )
}

const rulesResponse = {
  data: [
    {
      id: "nr1",
      workspace_id: "ws1",
      name: "task-done-notify",
      event_type: "task.completed",
      audience_type: "assignees",
      sink_id: "s1",
      enabled: true,
      created_at: 0,
      modified_at: 0,
    },
  ],
}

const sinksResponse = {
  data: [
    {
      id: "s1",
      workspace_id: "ws1",
      name: "feishu",
      type: "webhook",
      endpoint_mode: "static_url",
      enabled: true,
      timeout_seconds: 10,
      max_attempts: 5,
      max_concurrency: 1,
      created_at: 0,
      modified_at: 0,
    },
  ],
}

// event 触发源的模板变量视图。
const templateVarsResponse = {
  data: {
    triggers: [
      {
        trigger: "reminder",
        fields: [{ field: "body", vars: [{ name: "task.title", description: "任务标题", dynamic: false }] }],
      },
      {
        trigger: "event",
        fields: [
          { field: "endpoint", vars: [{ name: "workspace.id", description: "工作区 ID", dynamic: false }] },
          {
            field: "body",
            vars: [
              { name: "event.type", description: "事件类型", dynamic: false },
              { name: "actor.name", description: "操作者名称", dynamic: false },
            ],
          },
        ],
      },
    ],
  },
}

function mockFetch() {
  return vi.spyOn(globalThis, "fetch").mockImplementation((input: unknown) => {
    const url = typeof input === "string" ? input : (input as Request).url
    if (url.startsWith("/api/v1/notification-rules")) {
      return Promise.resolve(new Response(JSON.stringify(rulesResponse), { status: 200 }))
    }
    if (url.startsWith("/api/v1/notification-sinks")) {
      return Promise.resolve(new Response(JSON.stringify(sinksResponse), { status: 200 }))
    }
    if (url.startsWith("/api/v1/notification-template-vars")) {
      return Promise.resolve(new Response(JSON.stringify(templateVarsResponse), { status: 200 }))
    }
    return Promise.resolve(new Response(JSON.stringify({ data: [] }), { status: 200 }))
  })
}

describe("NotificationRuleList", () => {
  beforeEach(async () => {
    mockFetch()
    await i18n.changeLanguage("zh-CN")
  })

  it("lists rules with name, event, audience", async () => {
    renderList()
    await waitFor(() => {
      expect(screen.getByText("task-done-notify")).toBeTruthy()
    })
    expect(screen.getByText("task.completed")).toBeTruthy()
    expect(screen.getByText("assignees")).toBeTruthy()
  })

  it("shows write actions for write users", async () => {
    renderList(true)
    await waitFor(() => {
      expect(screen.getByText("task-done-notify")).toBeTruthy()
    })
    expect(screen.getByRole("button", { name: /新建通知规则/ })).toBeTruthy()
  })

  it("hides write actions for read-only users", async () => {
    renderList(false)
    await waitFor(() => {
      expect(screen.getByText("task-done-notify")).toBeTruthy()
    })
    expect(screen.queryByRole("button", { name: /新建通知规则/ })).toBeNull()
  })

  it("新建对话框选 sink 后显示 event 模板变量面板", async () => {
    renderList(true)
    await waitFor(() => {
      expect(screen.getByRole("button", { name: /新建通知规则/ })).toBeTruthy()
    })

    // 打开新建对话框。
    fireEvent.click(screen.getByRole("button", { name: /新建通知规则/ }))

    // 选中 sink（原生 select，用 aria-label 定位）。
    await waitFor(() => {
      expect(screen.getByRole("option", { name: /feishu/ })).toBeTruthy()
    })
    fireEvent.change(screen.getByLabelText(/目标 Sink/), { target: { value: "s1" } })

    // event 模板变量面板应出现，且包含 event.type（event 独有），不含 task.title。
    await waitFor(() => {
      expect(screen.getByText("event.type")).toBeTruthy()
    })
    expect(screen.queryByText("task.title")).toBeNull()
  })
})

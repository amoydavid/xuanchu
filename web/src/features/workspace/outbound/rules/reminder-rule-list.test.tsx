import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { fireEvent, render, screen, waitFor } from "@testing-library/react"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { ThemeProvider } from "@/components/theme-provider"
import { TooltipProvider } from "@/components/ui/tooltip"
import { i18n } from "@/i18n"

import { ReminderRuleList } from "./reminder-rule-list"

function renderList(canWrite = true) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={client}>
      <ThemeProvider>
        <TooltipProvider>
          <ReminderRuleList canWrite={canWrite} />
        </TooltipProvider>
      </ThemeProvider>
    </QueryClientProvider>
  )
}

const rulesResponse = {
  data: [
    {
      id: "rr1",
      workspace_id: "ws1",
      name: "daily-standup",
      schedule_type: "daily_at",
      schedule_value: "08:50",
      filter_source: "status:pending",
      audience_type: "assignees",
      sink_id: "s1",
      enabled: false,
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

// reminder 触发源的模板变量视图。
const templateVarsResponse = {
  data: {
    triggers: [
      {
        trigger: "reminder",
        fields: [
          { field: "endpoint", vars: [{ name: "workspace.id", description: "工作区 ID", dynamic: false }] },
          {
            field: "body",
            vars: [
              { name: "task.title", description: "任务标题", dynamic: false },
              { name: "reminder.sequence", description: "提醒序号", dynamic: false },
            ],
          },
        ],
      },
      { trigger: "event", fields: [{ field: "body", vars: [{ name: "event.type", description: "事件类型", dynamic: false }] }] },
    ],
  },
}

function mockFetch() {
  return vi.spyOn(globalThis, "fetch").mockImplementation((input: unknown) => {
    const url = typeof input === "string" ? input : (input as Request).url
    if (url.startsWith("/api/v1/reminder-rules")) {
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

describe("ReminderRuleList", () => {
  beforeEach(async () => {
    mockFetch()
    await i18n.changeLanguage("zh-CN")
  })

  it("lists reminder rules", async () => {
    renderList()
    await waitFor(() => {
      expect(screen.getByText("daily-standup")).toBeTruthy()
    })
    expect(screen.getByText(/daily_at: 08:50/)).toBeTruthy()
  })

  it("shows create button for write users", async () => {
    renderList(true)
    await waitFor(() => {
      expect(screen.getByText("daily-standup")).toBeTruthy()
    })
    expect(screen.getByRole("button", { name: /新建定时规则/ })).toBeTruthy()
  })

  it("hides create button for read-only", async () => {
    renderList(false)
    await waitFor(() => {
      expect(screen.getByText("daily-standup")).toBeTruthy()
    })
    expect(screen.queryByRole("button", { name: /新建定时规则/ })).toBeNull()
  })

  it("新建对话框选 sink 后显示 reminder 模板变量面板", async () => {
    renderList(true)
    await waitFor(() => {
      expect(screen.getByRole("button", { name: /新建定时规则/ })).toBeTruthy()
    })

    // 打开新建对话框。
    fireEvent.click(screen.getByRole("button", { name: /新建定时规则/ }))

    // 选中 sink（原生 select，用 aria-label 定位）。
    await waitFor(() => {
      expect(screen.getByRole("option", { name: /feishu/ })).toBeTruthy()
    })
    fireEvent.change(screen.getByLabelText(/目标 Sink/), { target: { value: "s1" } })

    // reminder 模板变量面板应出现，且包含 task.title（reminder 独有），不含 event.type。
    await waitFor(() => {
      expect(screen.getByText("task.title")).toBeTruthy()
    })
    expect(screen.queryByText("event.type")).toBeNull()
  })
})

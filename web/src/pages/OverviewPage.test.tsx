import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen } from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import type { MeResponse } from "@/features/workspace/session/useMe"
import { renderWithRouter } from "@/test/router-wrapper"
import { i18n } from "../i18n"
import { OverviewPage } from "./OverviewPage"

const me: MeResponse = {
  actor_type: "user",
  actor: { id: "u1", name: "alice", display_name: "张三" },
  effective_workspace: { slug: "local", name: "本地工作区" },
  effective_role: "owner",
  token: { scopes: ["*"], type: "pat" },
}

const home = {
  generated_at: 1_784_246_400,
  today: "2026-07-17",
  actor_type: "user",
  my_work: {
    open_count: 1,
    started_count: 1,
    overdue_count: 0,
    due_today_count: 1,
    high_priority_open_count: 1,
    items: [
      {
        task: {
          id: "task-1",
          uuid: "task-1",
          task_slug: "OPS-7",
          title: "确认上线清单",
          status: "pending",
          project: "ops",
        },
        reasons: ["started"],
      },
    ],
  },
  project_attention: [],
}

function renderOverview() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  })
  render(
    renderWithRouter(
      <QueryClientProvider client={queryClient}>
        <OverviewPage me={me} />
      </QueryClientProvider>
    )
  )
}

function mockFetch(
  responses: Record<string, unknown>,
  requested: string[] = []
) {
  vi.spyOn(globalThis, "fetch").mockImplementation((input) => {
    const url = String(input)
    requested.push(url)
    const prefix = Object.keys(responses).find((candidate) =>
      url.startsWith(candidate)
    )
    const data = prefix ? responses[prefix] : []
    return Promise.resolve(
      new Response(JSON.stringify({ data }), { status: 200 })
    )
  })
}

describe("OverviewPage", () => {
  beforeEach(async () => {
    sessionStorage.clear()
    localStorage.clear()
    await i18n.changeLanguage("zh-CN")
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it("loads the user home without audit, delivery, or paged task requests", async () => {
    const requested: string[] = []
    mockFetch(
      {
        "/api/v1/home": home,
        "/api/v1/config/effective": [],
        "/api/v1/projects": [],
      },
      requested
    )

    renderOverview()

    expect(
      await screen.findByRole("heading", { name: "我的今日" })
    ).toBeTruthy()
    expect(screen.getByText("确认上线清单")).toBeTruthy()
    expect(requested.some((url) => url.startsWith("/api/v1/home"))).toBe(true)
    expect(requested.some((url) => url.includes("/api/v1/audit"))).toBe(false)
    expect(
      requested.some((url) => url.includes("notification-deliveries"))
    ).toBe(false)
    expect(requested.some((url) => url.includes("tasks?limit=200"))).toBe(false)
    expect(screen.queryByText("当前操作者")).toBeNull()
    expect(screen.queryByText("最近失败投递")).toBeNull()
    expect(screen.queryByText("最近审计")).toBeNull()
  })

  it("shows effective workspace information by label and hides an empty section", async () => {
    mockFetch({
      "/api/v1/home": home,
      "/api/v1/projects": [],
      "/api/v1/config/effective": [
        {
          key: "notifications.default_sink",
          value: "••••••",
          source: "workspace",
          definition: {
            key: "notifications.default_sink",
            value_type: "string",
            allowed_scopes: ["workspace"],
            label: "默认通知渠道",
            description: "",
            enum_values: [],
            default_value: null,
            required: false,
            secret: true,
            show_on_console_home: true,
            created_at: 0,
            modified_at: 0,
          },
          show_on_console_home: true,
          missing_required: false,
        },
      ],
    })

    renderOverview()

    expect(
      await screen.findByRole("heading", { name: "工作区信息" })
    ).toBeTruthy()
    expect(screen.getByText("默认通知渠道")).toBeTruthy()
    expect(screen.getByText("notifications.default_sink")).toBeTruthy()
    expect(screen.getByText("••••••")).toBeTruthy()
    expect(screen.queryByText("没有标记为首页展示的配置")).toBeNull()
  })
})

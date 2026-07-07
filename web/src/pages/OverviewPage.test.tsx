import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor } from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import type { MeResponse } from "@/features/workspace/session/useMe"
import { renderWithRouter } from "@/test/router-wrapper"

import { i18n } from "../i18n"
import { OverviewPage } from "./OverviewPage"

function mockFetchByUrl(routes: Record<string, unknown>) {
  vi.spyOn(globalThis, "fetch").mockImplementation((input) => {
    const url = String(input)
    for (const prefix of Object.keys(routes)) {
      if (url.startsWith(prefix)) {
        return Promise.resolve(
          new Response(JSON.stringify({ data: routes[prefix] }), {
            status: 200,
          })
        )
      }
    }
    return Promise.resolve(
      new Response(JSON.stringify({ data: [] }), { status: 200 })
    )
  })
}

const meProps: MeResponse = {
  actor_type: "user",
  actor: { id: "u1", name: "local" },
  effective_workspace: { slug: "local" },
  effective_role: "owner",
  token: { scopes: ["*"], type: "pat" },
}

function renderOverview() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    renderWithRouter(
      <QueryClientProvider client={queryClient}>
        <OverviewPage me={meProps} />
      </QueryClientProvider>
    )
  )
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

  it("does not render fabricated delivery or audit rows when APIs return empty data", async () => {
    vi.spyOn(globalThis, "fetch").mockImplementation(() =>
      Promise.resolve(new Response(JSON.stringify({ data: [] }), { status: 200 }))
    )
    renderOverview()

    await waitFor(() => {
      expect(screen.getAllByText("暂无数据").length).toBeGreaterThan(0)
    })
    expect(screen.queryByText("dead_lettered")).toBeNull()
    expect(screen.queryByText("retry_wait")).toBeNull()
    expect(screen.queryByText("task.done")).toBeNull()
  })

  it("renders config overview section with label as primary text", async () => {
    mockFetchByUrl({
      "/api/v1/config/effective": [
        {
          key: "ads.roi_threshold",
          value: "1.8",
          source: "default",
          definition: {
            key: "ads.roi_threshold",
            value_type: "number",
            allowed_scopes: ["workspace"],
            label: "ROI 阈值",
            description: "",
            enum_values: [],
            default_value: "1.8",
            required: false,
            secret: false,
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
    // 等待数据行加载（query 异步）
    await waitFor(() => expect(screen.getByText("ads.roi_threshold")).toBeTruthy())
    // 主文本是 label
    expect(screen.getByText((content) => content.includes("ROI"))).toBeTruthy()
    expect(screen.getByText("默认")).toBeTruthy()
    expect(screen.getByText("去配置定义")).toBeTruthy()
  })

  it("shows empty hint when no home-display configs", async () => {
    mockFetchByUrl({ "/api/v1/config/effective": [] })
    renderOverview()
    await waitFor(() =>
      expect(screen.getByText("没有标记为首页展示的配置")).toBeTruthy()
    )
  })

  it("masks secret value as bullets", async () => {
    mockFetchByUrl({
      "/api/v1/config/effective": [
        {
          key: "ads.secret",
          value: "••••••",
          source: "workspace",
          definition: {
            key: "ads.secret",
            value_type: "string",
            allowed_scopes: ["workspace"],
            label: "Secret",
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
    await waitFor(() => expect(screen.getByText("Secret")).toBeTruthy())
    expect(screen.getByText("••••••")).toBeTruthy()
  })

  it("shows 未配置 for missing source", async () => {
    mockFetchByUrl({
      "/api/v1/config/effective": [
        {
          key: "ads.missing",
          value: null,
          source: "missing",
          definition: {
            key: "ads.missing",
            value_type: "string",
            allowed_scopes: ["workspace"],
            label: "默认上下文",
            description: "",
            enum_values: [],
            default_value: null,
            required: false,
            secret: false,
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
    await waitFor(() =>
      expect(screen.getAllByText("未配置").length).toBeGreaterThan(0)
    )
  })
})

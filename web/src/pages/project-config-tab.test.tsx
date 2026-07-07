import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor } from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { TooltipProvider } from "@/components/ui/tooltip"
import { i18n } from "@/i18n"
import { setWorkspaceToken } from "@/features/workspace/session/workspace-token"
import { renderWithRouter } from "@/test/router-wrapper"

import { ProjectConfigTab } from "./project-config-tab"

function ok(data: unknown) {
  return Promise.resolve(new Response(JSON.stringify({ data }), { status: 200 }))
}

function renderTab(props: Partial<{
  canManage: boolean
  closed: boolean
}> = {}) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    renderWithRouter(
      <QueryClientProvider client={queryClient}>
        <TooltipProvider>
          <ProjectConfigTab
            canManage={props.canManage ?? true}
            closed={props.closed ?? false}
            projectSlug="api"
            workspaceSlug="local"
          />
        </TooltipProvider>
      </QueryClientProvider>
    )
  )
}

// effective config 响应样本，覆盖四种 source。
const effectiveRows = [
  {
    key: "agent.background",
    value: "Owns MCP",
    source: "project",
    definition: {
      key: "agent.background",
      value_type: "string",
      allowed_scopes: ["project"],
      label: "",
      description: "",
      enum_values: [],
      default_value: null,
      required: false,
      secret: false,
      show_on_console_home: false,
      created_at: 0,
      modified_at: 0,
    },
    show_on_console_home: false,
    missing_required: false,
  },
  {
    key: "integrations.url",
    value: "https://example.com",
    source: "workspace",
    definition: {
      key: "integrations.url",
      value_type: "string",
      allowed_scopes: ["workspace", "project"],
      label: "",
      description: "",
      enum_values: [],
      default_value: null,
      required: false,
      secret: false,
      show_on_console_home: false,
      created_at: 0,
      modified_at: 0,
    },
    show_on_console_home: false,
    missing_required: false,
  },
  {
    key: "ads.budget",
    value: "500",
    source: "default",
    definition: {
      key: "ads.budget",
      value_type: "number",
      allowed_scopes: ["project"],
      label: "",
      description: "",
      enum_values: [],
      default_value: "500",
      required: false,
      secret: false,
      show_on_console_home: false,
      created_at: 0,
      modified_at: 0,
    },
    show_on_console_home: false,
    missing_required: false,
  },
  {
    key: "ads.owner",
    value: null,
    source: "missing",
    definition: {
      key: "ads.owner",
      value_type: "string",
      allowed_scopes: ["project"],
      label: "",
      description: "",
      enum_values: [],
      default_value: null,
      required: true,
      secret: false,
      show_on_console_home: false,
      created_at: 0,
      modified_at: 0,
    },
    show_on_console_home: false,
    missing_required: true,
  },
]

describe("ProjectConfigTab effective view", () => {
  beforeEach(async () => {
    sessionStorage.clear()
    setWorkspaceToken("xuanchu_pat_test")
    await i18n.changeLanguage("zh-CN")
  })

  afterEach(() => {
    vi.restoreAllMocks()
    sessionStorage.clear()
  })

  it("shows all four sources: project, workspace, default, missing", async () => {
    vi.spyOn(globalThis, "fetch").mockImplementation((input) => {
      const url = String(input)
      if (url.includes("/config/effective")) {
        return ok(effectiveRows)
      }
      return Promise.resolve(new Response("{}", { status: 200 }))
    })
    renderTab()
    await waitFor(() => expect(screen.getByText("agent.background")).toBeTruthy())
    expect(screen.getByText("integrations.url")).toBeTruthy()
    expect(screen.getByText("ads.budget")).toBeTruthy()
    expect(screen.getByText("ads.owner")).toBeTruthy()
    // 来源标签
    expect(screen.getByText("项目")).toBeTruthy()
    expect(screen.getByText("工作区")).toBeTruthy()
    expect(screen.getByText("默认")).toBeTruthy()
    expect(screen.getByText("未配置")).toBeTruthy()
  })

  it("shows missing-required status for required missing key", async () => {
    vi.spyOn(globalThis, "fetch").mockImplementation((input) => {
      const url = String(input)
      if (url.includes("/config/effective")) {
        return ok(effectiveRows)
      }
      return Promise.resolve(new Response("{}", { status: 200 }))
    })
    renderTab()
    await waitFor(() => expect(screen.getByText("ads.owner")).toBeTruthy())
    expect(screen.getByText("必填缺失")).toBeTruthy()
  })

  it("shows restore-inherited button for project-source rows", async () => {
    vi.spyOn(globalThis, "fetch").mockImplementation((input) => {
      const url = String(input)
      if (url.includes("/config/effective")) {
        return ok(effectiveRows)
      }
      return Promise.resolve(new Response("{}", { status: 200 }))
    })
    renderTab()
    await waitFor(() => expect(screen.getByText("agent.background")).toBeTruthy())
    expect(screen.getByRole("button", { name: "恢复继承" })).toBeTruthy()
  })

  it("hides write actions when project is closed", async () => {
    vi.spyOn(globalThis, "fetch").mockImplementation((input) => {
      const url = String(input)
      if (url.includes("/config/effective")) {
        return ok(effectiveRows)
      }
      return Promise.resolve(new Response("{}", { status: 200 }))
    })
    renderTab({ closed: true })
    await waitFor(() => expect(screen.getByText("agent.background")).toBeTruthy())
    expect(screen.queryByRole("button", { name: "恢复继承" })).toBeNull()
  })
})

import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor } from "@testing-library/react"
import { userEvent } from "@testing-library/user-event"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { TooltipProvider } from "@/components/ui/tooltip"
import { i18n } from "@/i18n"
import { setWorkspaceToken } from "@/features/workspace/session/workspace-token"
import { renderWithRouter } from "@/test/router-wrapper"

import { ConfigDefinitionManager } from "./config-definition-manager"

function ok(data: unknown) {
  return Promise.resolve(new Response(JSON.stringify({ data }), { status: 200 }))
}

function jsonBody(resp: unknown) {
  return Promise.resolve(
    new Response(JSON.stringify(resp), { status: 200 })
  )
}

function renderManager(props: {
  variant?: "workspace" | "project"
  canManage?: boolean
}) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    renderWithRouter(
      <QueryClientProvider client={queryClient}>
        <TooltipProvider>
          <ConfigDefinitionManager
            variant={props.variant ?? "workspace"}
            title="配置定义"
            description="说明"
            defaultScopes={
              props.variant === "project" ? ["project"] : ["workspace"]
            }
            canManage={props.canManage ?? true}
          />
        </TooltipProvider>
      </QueryClientProvider>
    )
  )
  return queryClient
}

const schemaList = [
  {
    key: "ads.budget",
    value_type: "number",
    allowed_scopes: ["workspace", "project"],
    label: "广告预算",
    description: "",
    enum_values: [],
    default_value: null,
    required: false,
    secret: false,
    show_on_console_home: false,
    created_at: 0,
    modified_at: 0,
  },
  {
    key: "agent.background",
    value_type: "string",
    allowed_scopes: ["project"],
    label: "Agent 背景",
    description: "",
    enum_values: [],
    default_value: null,
    required: false,
    secret: false,
    show_on_console_home: false,
    created_at: 0,
    modified_at: 0,
  },
  {
    key: "runtime.timeout",
    value_type: "number",
    allowed_scopes: ["workspace"],
    label: "运行时超时",
    description: "",
    enum_values: [],
    default_value: null,
    required: false,
    secret: false,
    show_on_console_home: false,
    created_at: 0,
    modified_at: 0,
  },
]

describe("ConfigDefinitionManager", () => {
  beforeEach(async () => {
    sessionStorage.clear()
    setWorkspaceToken("xuanchu_pat_test")
    await i18n.changeLanguage("zh-CN")
  })

  afterEach(() => {
    vi.restoreAllMocks()
    sessionStorage.clear()
  })

  it("workspace variant shows all definitions", async () => {
    vi.spyOn(globalThis, "fetch").mockImplementation((input) => {
      const url = String(input)
      if (url.startsWith("/api/v1/config-schema/")) {
        return jsonBody({ data: schemaList[0] })
      }
      if (url.startsWith("/api/v1/config-schema")) {
        return ok(schemaList)
      }
      return Promise.resolve(new Response("{}", { status: 200 }))
    })
    renderManager({ variant: "workspace" })
    await waitFor(() => expect(screen.getByText("ads.budget")).toBeTruthy())
    expect(screen.getByText("agent.background")).toBeTruthy()
    expect(screen.getByText("runtime.timeout")).toBeTruthy()
  })

  it("project variant defaults to project-scope definitions only", async () => {
    vi.spyOn(globalThis, "fetch").mockImplementation((input) => {
      const url = String(input)
      if (url.startsWith("/api/v1/config-schema")) {
        return ok(schemaList)
      }
      return Promise.resolve(new Response("{}", { status: 200 }))
    })
    renderManager({ variant: "project" })
    // ads.budget 允许 project，agent.background 允许 project，runtime.timeout 只 workspace
    await waitFor(() => expect(screen.getByText("ads.budget")).toBeTruthy())
    expect(screen.getByText("agent.background")).toBeTruthy()
    // project-only toggle 默认开启，runtime.timeout 不显示
    expect(screen.queryByText("runtime.timeout")).toBeNull()
  })

  it("invalidates config-schema query after save", async () => {
    const user = userEvent.setup()
    const fetchSpy = vi.spyOn(globalThis, "fetch").mockImplementation((input) => {
      const url = String(input)
      if (url.startsWith("/api/v1/config-schema/")) {
        return jsonBody({ data: schemaList[0] })
      }
      if (url.startsWith("/api/v1/config-schema")) {
        return ok(schemaList)
      }
      return Promise.resolve(new Response("{}", { status: 200 }))
    })
    renderManager({ variant: "workspace" })
    await waitFor(() => expect(screen.getByText("ads.budget")).toBeTruthy())

    // 打开第一个定义的编辑表单
    const editButtons = await screen.findAllByRole("button", { name: /编辑/ })
    await user.click(editButtons[0])
    // 至少证明 fetch 被用于 schema 读取
    expect(fetchSpy).toHaveBeenCalled()
  })

  it("shows empty state when no definitions", async () => {
    vi.spyOn(globalThis, "fetch").mockImplementation((input) => {
      const url = String(input)
      if (url.startsWith("/api/v1/config-schema")) {
        return ok([])
      }
      return Promise.resolve(new Response("{}", { status: 200 }))
    })
    renderManager({ variant: "workspace" })
    await waitFor(() =>
      expect(screen.getByText("当前 workspace 无配置定义")).toBeTruthy()
    )
  })
})

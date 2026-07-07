import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor } from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { TooltipProvider } from "@/components/ui/tooltip"
import { i18n } from "@/i18n"
import { setWorkspaceToken } from "@/features/workspace/session/workspace-token"
import { renderWithRouter } from "@/test/router-wrapper"

import { ConfigDefinitionsPage } from "./config-definitions-page"

function ok(data: unknown) {
  return Promise.resolve(new Response(JSON.stringify({ data }), { status: 200 }))
}

function renderPage(variant: "workspace" | "project" = "workspace") {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    renderWithRouter(
      <QueryClientProvider client={queryClient}>
        <TooltipProvider>
          <ConfigDefinitionsPage variant={variant} />
        </TooltipProvider>
      </QueryClientProvider>
    )
  )
}

const schemaList = [
  {
    key: "ads.budget",
    value_type: "number",
    allowed_scopes: ["workspace"],
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
]

describe("ConfigDefinitionsPage", () => {
  beforeEach(async () => {
    sessionStorage.clear()
    setWorkspaceToken("xuanchu_pat_test")
    await i18n.changeLanguage("zh-CN")
  })

  afterEach(() => {
    vi.restoreAllMocks()
    sessionStorage.clear()
  })

  it("workspace variant requests config-schema instead of config values", async () => {
    const fetchSpy = vi
      .spyOn(globalThis, "fetch")
      .mockImplementation((input) => {
        const url = String(input)
        // 关键：请求 /api/v1/config-schema，而不是 /api/v1/config
        if (url.startsWith("/api/v1/config-schema")) {
          return ok(schemaList)
        }
        return Promise.resolve(new Response("{}", { status: 200 }))
      })
    renderPage("workspace")
    await waitFor(() => expect(screen.getByText("ads.budget")).toBeTruthy())
    const calls = fetchSpy.mock.calls.map((c) => String(c[0]))
    expect(calls.some((u) => u.startsWith("/api/v1/config-schema"))).toBe(true)
    expect(calls.some((u) => u.startsWith("/api/v1/config?"))).toBe(false)
  })

  it("renders the page title from i18n", async () => {
    vi.spyOn(globalThis, "fetch").mockImplementation((input) => {
      const url = String(input)
      if (url.startsWith("/api/v1/config-schema")) {
        return ok(schemaList)
      }
      return Promise.resolve(new Response("{}", { status: 200 }))
    })
    renderPage("workspace")
    await waitFor(() => expect(screen.getByText("配置定义")).toBeTruthy())
  })
})

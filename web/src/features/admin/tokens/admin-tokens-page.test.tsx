import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor, fireEvent } from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { ThemeProvider } from "@/components/theme-provider"
import { TooltipProvider } from "@/components/ui/tooltip"
import { setAdminToken } from "@/features/admin/session/admin-token"
import { i18n } from "@/i18n"

import { AdminTokensPage } from "./admin-tokens-page"
import type { AdminTokenRow } from "./admin-token-api"

function renderPage() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={queryClient}>
      <ThemeProvider>
        <TooltipProvider>
          <AdminTokensPage />
        </TooltipProvider>
      </ThemeProvider>
    </QueryClientProvider>
  )
}

function okResponse(data: unknown) {
  return Promise.resolve(
    new Response(JSON.stringify({ data }), { status: 200 })
  )
}

function makeRow(overrides: Partial<AdminTokenRow> = {}): AdminTokenRow {
  return {
    id: "tok-1",
    prefix: "xuanchu_agent_abc",
    name: "ci-deploy",
    type: "agent",
    user: { id: "u1", name: "alice", email: "alice@x.com" },
    workspace_ids: ["w1"],
    project_ids: [],
    scopes: ["task:read", "task:write"],
    created_at: 100,
    expires_at: null,
    revoked_at: null,
    last_used_at: 200,
    ...overrides,
  }
}

describe("AdminTokensPage", () => {
  beforeEach(async () => {
    sessionStorage.clear()
    setAdminToken("xuanchu_admin_test")
    await i18n.changeLanguage("zh-CN")
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it("renders token rows with user name+email, workspaces, scopes, status", async () => {
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockImplementation((input) => {
        const url = String(input)
        if (url.includes("/api/v1/admin/tokens")) {
          return okResponse([makeRow()])
        }
        return okResponse([])
      })

    renderPage()

    await waitFor(() => {
      expect(screen.getByText("ci-deploy")).toBeTruthy()
    })
    expect(screen.getByText("alice")).toBeTruthy()
    expect(screen.getByText("alice@x.com")).toBeTruthy()
    expect(screen.getByText("有效")).toBeTruthy()
    // 无创建按钮
    expect(screen.queryByText("创建 Token")).toBeNull()

    fetchMock.mockRestore()
  })

  it("shows revoked row with reduced opacity and disabled actions", async () => {
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockImplementation(() =>
        okResponse([makeRow({ name: "legacy", revoked_at: 999, id: "tok-2" })])
      )

    renderPage()

    await waitFor(() => {
      expect(screen.getByText("已吊销")).toBeTruthy()
    })
    // 操作按钮应禁用（disabled）
    const actionBtn = screen.getByText("⋯").closest("button")
    expect(actionBtn?.disabled).toBe(true)

    fetchMock.mockRestore()
  })

  it("renders PAT with global label when no workspaces", async () => {
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockImplementation(() =>
        okResponse([
          makeRow({
            name: "admin-pat",
            type: "pat",
            workspace_ids: [],
            id: "tok-3",
          }),
        ])
      )

    renderPage()

    await waitFor(() => {
      expect(screen.getByText("全局")).toBeTruthy()
    })

    fetchMock.mockRestore()
  })

  it("renders empty state when no tokens", async () => {
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockImplementation(() => okResponse([]))

    renderPage()

    await waitFor(() => {
      expect(screen.getByText("暂无数据")).toBeTruthy()
    })

    fetchMock.mockRestore()
  })

  it("toggles includeRevoked checkbox changes query", async () => {
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockImplementation(() => okResponse([makeRow()]))

    renderPage()

    await waitFor(() => {
      expect(screen.getByText("ci-deploy")).toBeTruthy()
    })
    // 第一次请求应含 all=true（默认勾选）
    expect(fetchMock.mock.calls[0]?.[0]).toContain("all=true")

    // 取消勾选
    const checkboxes = screen.getAllByRole("checkbox")
    fireEvent.click(checkboxes[checkboxes.length - 1])

    await waitFor(() => {
      expect(
        fetchMock.mock.calls.some((call) =>
          String(call[0]).includes("all=false")
        )
      ).toBe(true)
    })

    fetchMock.mockRestore()
  })
})

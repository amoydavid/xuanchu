import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor } from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { ThemeProvider } from "@/components/theme-provider"
import { TooltipProvider } from "@/components/ui/tooltip"
import { setWorkspaceToken } from "@/features/workspace/session/workspace-token"
import { i18n } from "@/i18n"

import { TokensPage } from "./tokens-page"
import type { TokenRow } from "./token-api"

function renderPage() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={queryClient}>
      <ThemeProvider>
        <TooltipProvider>
          <TokensPage />
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

function makeToken(overrides: Partial<TokenRow> = {}): TokenRow {
  return {
    id: "tok-1",
    prefix: "xuanchu_pat_abc",
    name: "ci-deploy",
    type: "agent",
    user: { id: "u1", name: "local" },
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

describe("TokensPage", () => {
  beforeEach(async () => {
    sessionStorage.clear()
    setWorkspaceToken("xuanchu_pat_test")
    await i18n.changeLanguage("zh-CN")
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it("renders token rows with name, type, scopes count and status", async () => {
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockImplementation((input) => {
        const url = String(input)
        if (url.includes("/api/v1/me")) {
          return okResponse({
            actor: { name: "local" },
            token: { type: "pat", scopes: ["token:read", "token:write"] },
            effective_workspace: { slug: "local" },
            effective_role: "owner",
          })
        }
        if (url.includes("/api/v1/tokens")) {
          return okResponse([makeToken()])
        }
        return okResponse([])
      })

    renderPage()

    await waitFor(() => {
      expect(screen.getByText("ci-deploy")).toBeTruthy()
    })

    // type 显示为 Badge
    expect(screen.getByText("agent")).toBeTruthy()
    // scopes 数量（格式「2 权限范围 (Scope)」）
    expect(screen.getByText(/2 .*权限范围/)).toBeTruthy()
    // 状态：有效
    expect(screen.getByText("有效")).toBeTruthy()
    // 创建按钮
    expect(screen.getByText("创建 Token")).toBeTruthy()

    fetchMock.mockRestore()
  })

  it("shows revoked row with reduced opacity (revoked status badge)", async () => {
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockImplementation((input) => {
        const url = String(input)
        if (url.includes("/api/v1/me")) {
          return okResponse({
            actor: { name: "local" },
            token: { type: "pat", scopes: ["token:read"] },
            effective_workspace: { slug: "local" },
            effective_role: "owner",
          })
        }
        if (url.includes("/api/v1/tokens")) {
          return okResponse([
            makeToken({ name: "legacy", revoked_at: 999, id: "tok-2" }),
          ])
        }
        return okResponse([])
      })

    renderPage()

    await waitFor(() => {
      expect(screen.getByText("已吊销")).toBeTruthy()
    })
    expect(screen.getByText("legacy")).toBeTruthy()

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

  it("shows error message on API failure", async () => {
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockImplementation((input) => {
        const url = String(input)
        if (url.includes("/api/v1/me")) {
          return okResponse({
            actor: { name: "local" },
            token: { type: "pat", scopes: [] },
            effective_workspace: { slug: "local" },
            effective_role: "owner",
          })
        }
        return Promise.resolve(
          new Response(
            JSON.stringify({ error: { code: "token_not_found" } }),
            { status: 500 }
          )
        )
      })

    renderPage()

    await waitFor(() => {
      expect(screen.getByText(/发生错误/)).toBeTruthy()
    })

    fetchMock.mockRestore()
  })
})

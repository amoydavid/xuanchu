import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor } from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { ThemeProvider } from "@/components/theme-provider"
import { TooltipProvider } from "@/components/ui/tooltip"
import {
  setAdminActingContext,
  setAdminActingToken,
  setWorkspaceToken,
} from "@/features/workspace/session/workspace-token"
import { i18n } from "@/i18n"
import { renderWithRouter } from "@/test/router-wrapper"

import { WorkspaceRootRoute } from "./WorkspaceRootRoute"

function renderRoute() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const result = render(
    renderWithRouter(
      <QueryClientProvider client={queryClient}>
        <ThemeProvider>
          <TooltipProvider>
            <WorkspaceRootRoute />
          </TooltipProvider>
        </ThemeProvider>
      </QueryClientProvider>
    )
  )
  return { result, queryClient }
}

function meResponse() {
  return Promise.resolve(
    new Response(
      JSON.stringify({
        data: {
          actor: { id: "u1", name: "alice", email: "alice@example.com" },
          token: { id: "act-1", name: "admin-acting-ops", type: "admin_acting", scopes: ["task:read"] },
          visible_workspaces: [{ id: "ws-1", slug: "dajee" }],
          effective_workspace: { id: "ws-1", slug: "dajee" },
          effective_role: "owner",
        },
      }),
      { status: 200 }
    )
  )
}

describe("WorkspaceRootRoute acting mode", () => {
  beforeEach(async () => {
    sessionStorage.clear()
    await i18n.changeLanguage("zh-CN")
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it("treats acting token as signed-in and does not show login page", async () => {
    // 只设置 acting token + context，不设置普通 workspace token。
    setAdminActingToken("xuanchu_act_test")
    setAdminActingContext({
      workspaceSlug: "dajee",
      workspaceName: "Dajee",
      actorName: "alice",
      role: "owner",
      adminTokenName: "ops",
    })
    const fetchMock = vi.spyOn(globalThis, "fetch").mockImplementation(() => meResponse())

    renderRoute()

    // 不应该出现登录页文案；应该渲染出 AppShell 的 actor 信息。
    await waitFor(() => {
      expect(screen.getByText(/alice/)).toBeTruthy()
    })
    expect(screen.queryByText(/使用璇础 token 登录/)).toBeNull()

    fetchMock.mockRestore()
  })

  it("shows login page when neither workspace nor acting token exists", async () => {
    renderRoute()

    await waitFor(() => {
      expect(screen.queryByText(/使用璇础 token 登录/)).toBeTruthy()
    })
  })

  it("treats normal workspace token as signed-in (regression)", async () => {
    setWorkspaceToken("xuanchu_pat_test")
    const fetchMock = vi.spyOn(globalThis, "fetch").mockImplementation(() => meResponse())

    renderRoute()

    await waitFor(() => {
      expect(screen.getByText(/alice/)).toBeTruthy()
    })

    fetchMock.mockRestore()
  })
})

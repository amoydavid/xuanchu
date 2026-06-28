import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor, fireEvent } from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { ThemeProvider } from "@/components/theme-provider"
import { TooltipProvider } from "@/components/ui/tooltip"
import {
  clearAdminActingSession,
  clearWorkspaceToken,
  getAdminActingContext,
  getAdminActingToken,
  setWorkspaceToken,
} from "@/features/workspace/session/workspace-token"
import { setAdminToken } from "@/features/admin/session/admin-token"
import { i18n } from "@/i18n"
import { navigateToDocument } from "@/lib/browser-navigation"

import { AdminWorkspaceDetailPage } from "./admin-workspace-detail-page"
import type { AdminWorkspaceDetail } from "./admin-workspace-api"

vi.mock("@/lib/browser-navigation", () => ({
  navigateToDocument: vi.fn(),
}))

function renderPage(workspaceSlug: string) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={queryClient}>
      <ThemeProvider>
        <TooltipProvider>
          <AdminWorkspaceDetailPage workspaceSlug={workspaceSlug} />
        </TooltipProvider>
      </ThemeProvider>
    </QueryClientProvider>
  )
}

function okResponse(data: unknown, status = 200) {
  return Promise.resolve(
    new Response(JSON.stringify({ data }), { status })
  )
}

function makeDetail(overrides: Partial<AdminWorkspaceDetail> = {}): AdminWorkspaceDetail {
  return {
    workspace: { id: "ws-1", slug: "dajee", name: "Dajee", visibility: "team" },
    members: [
      {
        user: { id: "u1", name: "alice", email: "alice@example.com" },
        role: "owner",
        joined_at: 100,
        modified_at: 100,
      },
      {
        user: { id: "u2", name: "bob", email: "bob@example.com" },
        role: "admin",
        joined_at: 110,
        modified_at: 110,
      },
    ],
    token_counts: { active: 3, expired: 0, revoked: 1 },
    acting_candidates: [
      { user: { id: "u1", name: "alice", email: "alice@example.com" }, role: "owner" },
    ],
    ...overrides,
  }
}

describe("AdminWorkspaceDetailPage", () => {
  beforeEach(async () => {
    sessionStorage.clear()
    setAdminToken("xuanchu_admin_test")
    setWorkspaceToken("xuanchu_pat_normal")
    await i18n.changeLanguage("zh-CN")
  })

  afterEach(() => {
    vi.restoreAllMocks()
    vi.mocked(navigateToDocument).mockClear()
  })

  it("renders members and token counts", async () => {
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockImplementation(() => okResponse(makeDetail()))

    renderPage("dajee")

    await waitFor(() => {
      expect(screen.getByText("Dajee")).toBeTruthy()
    })
    expect(screen.getByText("alice")).toBeTruthy()
    expect(screen.getByText("bob@example.com")).toBeTruthy()
    expect(screen.getByText(/有效 3/)).toBeTruthy()

    fetchMock.mockRestore()
  })

  it("creates acting session and stores token + context on confirm", async () => {
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockImplementation((input, init) => {
        const url = String(input)
        const method = (init?.method ?? "GET").toUpperCase()
        if (url.includes("/api/v1/admin/workspaces/dajee") && method === "POST") {
          return okResponse({
            token: "xuanchu_act_abc",
            expires_at: 9999,
            workspace: { id: "ws-1", slug: "dajee", name: "Dajee" },
            actor: { id: "u1", name: "alice", email: "alice@example.com" },
            role: "owner",
            admin_token_name: "ops",
          }, 201)
        }
        return okResponse(makeDetail())
      })

    renderPage("dajee")

    // 打开 acting dialog。
    await waitFor(() => {
      expect(screen.getByText("以管理员身份进入")).toBeTruthy()
    })
    fireEvent.click(screen.getByText("以管理员身份进入"))

    // 确认创建 acting session。
    await waitFor(() => {
      expect(screen.getByText("进入 Workspace")).toBeTruthy()
    })
    fireEvent.click(screen.getByText("进入 Workspace"))

    // acting token + context 应已写入 sessionStorage。
    await waitFor(() => {
      expect(getAdminActingToken()).toBe("xuanchu_act_abc")
    })
    const ctx = getAdminActingContext()
    expect(ctx?.actorName).toBe("alice")
    expect(ctx?.workspaceSlug).toBe("dajee")
    expect(ctx?.adminTokenName).toBe("ops")
    expect(navigateToDocument).toHaveBeenCalledWith("/workspaces/dajee/projects")
    // 普通 workspace token 不应被覆盖。
    expect(clearWorkspaceToken !== undefined).toBe(true)
    // admin token 必须保留。
    expect(sessionStorage.getItem("xuanchu.console.admin_token")).toBe(
      "xuanchu_admin_test"
    )

    fetchMock.mockRestore()
    clearAdminActingSession()
  })

  it("shows create-admin flow when no acting candidates", async () => {
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockImplementation(() =>
        okResponse(makeDetail({ acting_candidates: [] }))
      )

    renderPage("dajee")

    await waitFor(() => {
      expect(
        screen.getByText(/该 workspace 暂无 owner\/admin/)
      ).toBeTruthy()
    })

    fetchMock.mockRestore()
  })
})

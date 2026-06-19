import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor, fireEvent } from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { ThemeProvider } from "@/components/theme-provider"
import { TooltipProvider } from "@/components/ui/tooltip"
import { setAdminToken } from "@/features/admin/session/admin-token"
import { i18n } from "@/i18n"

import { AdminWorkspacesPage } from "./admin-workspaces-page"
import type { AdminWorkspaceSummary } from "./admin-workspace-api"

function renderPage() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={queryClient}>
      <ThemeProvider>
        <TooltipProvider>
          <AdminWorkspacesPage />
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

function makeRow(overrides: Partial<AdminWorkspaceSummary> = {}): AdminWorkspaceSummary {
  return {
    id: "ws-1",
    slug: "dajee",
    name: "Dajee",
    description: "",
    visibility: "team",
    created_by: { id: "u1", name: "alice", email: "alice@example.com" },
    member_counts: { owner: 1, admin: 2, member: 5, viewer: 3 },
    token_counts: { active: 3, expired: 0, revoked: 1 },
    archived_at: null,
    created_at: 100,
    modified_at: 100,
    ...overrides,
  }
}

describe("AdminWorkspacesPage", () => {
  beforeEach(async () => {
    sessionStorage.clear()
    setAdminToken("xuanchu_admin_test")
    await i18n.changeLanguage("zh-CN")
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it("renders workspace rows with slug, member counts, and token counts", async () => {
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockImplementation(() => okResponse([makeRow()]))

    renderPage()

    await waitFor(() => {
      expect(screen.getByText("dajee")).toBeTruthy()
    })
    expect(screen.getByText("Dajee")).toBeTruthy()
    // token 计数必须出现（格式：有效 N · ...）。
    expect(screen.getByText(/有效 3/)).toBeTruthy()

    fetchMock.mockRestore()
  })

  it("toggles archived filter changes the query param", async () => {
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockImplementation(() => okResponse([makeRow()]))

    renderPage()

    await waitFor(() => {
      expect(screen.getByText("dajee")).toBeTruthy()
    })
    // 默认不含 all=true。
    expect(fetchMock.mock.calls[0]?.[0]).toContain("all=false")

    // 勾选「显示已归档」。
    const checkbox = screen.getAllByRole("checkbox")
    fireEvent.click(checkbox[checkbox.length - 1])

    await waitFor(() => {
      expect(
        fetchMock.mock.calls.some((call) =>
          String(call[0]).includes("all=true")
        )
      ).toBe(true)
    })

    fetchMock.mockRestore()
  })

  it("renders archived workspace when toggle is on", async () => {
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockImplementation(() =>
        okResponse([makeRow({ slug: "legacy", name: "Legacy", archived_at: 999 })])
      )

    renderPage()
    await waitFor(() => {
      expect(screen.getByText("legacy")).toBeTruthy()
    })

    fetchMock.mockRestore()
  })
})

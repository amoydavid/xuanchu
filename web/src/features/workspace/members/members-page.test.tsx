import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { ThemeProvider } from "@/components/theme-provider"
import { TooltipProvider } from "@/components/ui/tooltip"
import { setWorkspaceToken } from "@/features/workspace/session/workspace-token"
import { i18n } from "@/i18n"

import { MembersPage } from "./members-page"

function renderPage(workspaceSlug = "dajee") {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={queryClient}>
      <ThemeProvider>
        <TooltipProvider>
          <MembersPage workspaceSlug={workspaceSlug} />
        </TooltipProvider>
      </ThemeProvider>
    </QueryClientProvider>
  )
}

function okResponse(data: unknown, status = 200) {
  return Promise.resolve(new Response(JSON.stringify({ data }), { status }))
}

describe("MembersPage", () => {
  beforeEach(async () => {
    sessionStorage.clear()
    setWorkspaceToken("xuanchu_pat_test")
    await i18n.changeLanguage("zh-CN")
  })

  afterEach(() => {
    vi.restoreAllMocks()
    sessionStorage.clear()
  })

  it("updates a member display name from the workspace members page", async () => {
    const user = userEvent.setup()
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockImplementation((input, init) => {
        const url = String(input)
        const method = (init?.method ?? "GET").toUpperCase()
        if (url.includes("/api/v1/users/u2") && method === "PATCH") {
          return okResponse({
            id: "u2",
            name: "bob",
            display_name: "李四",
            email: "bob@example.com",
            external_ids: [],
            active: true,
            created_at: 1,
            modified_at: 2,
          })
        }
        return okResponse([
          {
            user_id: "u1",
            name: "alice",
            display_name: "Alice Chen",
            email: "alice@example.com",
            role: "owner",
            joined_at: 100,
            modified_at: 100,
          },
          {
            user_id: "u2",
            name: "bob",
            display_name: "Bob Li",
            email: "bob@example.com",
            role: "admin",
            joined_at: 110,
            modified_at: 110,
          },
        ])
      })

    renderPage()

    await screen.findByText("Bob Li")
    await user.click(screen.getByRole("button", { name: "编辑显示姓名 Bob Li" }))
    await user.clear(screen.getByLabelText("显示姓名"))
    await user.type(screen.getByLabelText("显示姓名"), "李四")
    await user.click(screen.getByRole("button", { name: "保存" }))

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        expect.stringContaining("/api/v1/users/u2"),
        expect.objectContaining({
          method: "PATCH",
          body: JSON.stringify({ display_name: "李四" }),
        })
      )
    })
  })
})

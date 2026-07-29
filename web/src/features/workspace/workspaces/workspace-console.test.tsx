import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor } from "@testing-library/react"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { ThemeProvider } from "@/components/theme-provider"
import { TooltipProvider } from "@/components/ui/tooltip"
import { i18n } from "@/i18n"
import { renderWithRouter } from "@/test/router-wrapper"

import { WorkspaceConsole } from "./workspace-console"

// 包裹 Provider 栈，配合 renderWithRouter 渲染（组件含 <Link>）。
function renderConsole(canWrite = true) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    renderWithRouter(
      <QueryClientProvider client={client}>
        <ThemeProvider>
          <TooltipProvider>
            <WorkspaceConsole canWrite={canWrite} />
          </TooltipProvider>
        </ThemeProvider>
      </QueryClientProvider>
    )
  )
}

describe("WorkspaceConsole", () => {
  beforeEach(async () => {
    vi.spyOn(globalThis, "fetch").mockImplementation((input) => {
      const url = typeof input === "string" ? input : (input as Request).url
      if (url.includes("/credentials/current")) {
        return Promise.resolve(
          new Response(
            JSON.stringify({
              data: {
                actor_type: "user",
                actor: { id: "u1", name: "U" },
                token: { type: "xuanchu_pat", scopes: ["config:write"] },
                effective_workspace: { slug: "acme", name: "ACME" },
                effective_role: "owner",
              },
            }),
            { status: 200 }
          )
        )
      }
      return Promise.resolve(
        new Response(
          JSON.stringify({
            data: [
              { id: "w1", slug: "acme", name: "ACME", archived: false },
              { id: "w2", slug: "sandbox", name: "Sandbox", archived: true },
            ],
          }),
          { status: 200 }
        )
      )
    })
    await i18n.changeLanguage("zh-CN")
  })

  it("lists workspaces and shows archive action for active, restore (disabled) for archived", async () => {
    renderConsole()
    await waitFor(() => {
      expect(screen.getByText("acme")).toBeTruthy()
    })
    expect(screen.getByText("sandbox")).toBeTruthy()
    // active 行有归档按钮
    expect(screen.getByRole("button", { name: /归档/ })).toBeTruthy()
    // archived 行的恢复按钮置灰
    const restoreBtn = screen.getByRole("button", { name: /恢复/ })
    expect(restoreBtn).toBeTruthy()
    expect((restoreBtn as HTMLButtonElement).disabled).toBe(true)
  })

  it("hides archive action when canWrite is false", async () => {
    renderConsole(false)
    await waitFor(() => {
      expect(screen.getByText("acme")).toBeTruthy()
    })
    expect(screen.queryByRole("button", { name: /归档/ })).toBeNull()
  })

  it("shows config link only on effective workspace row", async () => {
    renderConsole()
    await waitFor(() => {
      expect(screen.getByText("acme")).toBeTruthy()
    })
    // effective 行（acme）有「配置」链接，指向 /workspaces/acme/config
    const configLink = screen.getByRole("link", { name: /配置/ })
    expect(configLink.getAttribute("href")).toContain("/workspaces/acme/config")
    // 非当前行（sandbox）无「配置」链接：整页只有一个配置链接
    const allConfigLinks = screen.getAllByRole("link", { name: /配置/ })
    expect(allConfigLinks.length).toBe(1)
  })
})

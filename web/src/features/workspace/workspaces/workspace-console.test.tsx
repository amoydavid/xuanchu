import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor } from "@testing-library/react"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { ThemeProvider } from "@/components/theme-provider"
import { TooltipProvider } from "@/components/ui/tooltip"
import { i18n } from "@/i18n"

import { WorkspaceConsole } from "./workspace-console"

function renderConsole(canWrite = true) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={client}>
      <ThemeProvider>
        <TooltipProvider>
          <WorkspaceConsole canWrite={canWrite} />
        </TooltipProvider>
      </ThemeProvider>
    </QueryClientProvider>
  )
}

describe("WorkspaceConsole", () => {
  beforeEach(async () => {
    vi.spyOn(globalThis, "fetch").mockImplementation(() =>
      Promise.resolve(
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
    )
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
})

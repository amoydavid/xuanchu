import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor } from "@testing-library/react"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { ThemeProvider } from "@/components/theme-provider"
import { TooltipProvider } from "@/components/ui/tooltip"
import { i18n } from "@/i18n"

import { OutboundConsole } from "./outbound-console"

function renderConsole(initialTab: "hooks" | "notification-rules" | "sinks" = "hooks") {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={client}>
      <ThemeProvider>
        <TooltipProvider>
          <OutboundConsole initialTab={initialTab} workspaceSlug="dajee" />
        </TooltipProvider>
      </ThemeProvider>
    </QueryClientProvider>
  )
}

const meResponse = {
  data: {
    actor: { id: "u1", name: "alice" },
    actor_type: "user",
    token: { type: "pat", scopes: ["hook:read", "hook:write", "notification:write"] },
    effective_workspace: { slug: "dajee", name: "dajee" },
    effective_role: "owner",
  },
}

function mockFetch() {
  return vi.spyOn(globalThis, "fetch").mockImplementation((input: unknown) => {
    const url = typeof input === "string" ? input : (input as Request).url
    if (url.includes("/api/v1/credentials/current")) {
      return Promise.resolve(new Response(JSON.stringify(meResponse), { status: 200 }))
    }
    return Promise.resolve(new Response(JSON.stringify({ data: [] }), { status: 200 }))
  })
}

describe("OutboundConsole", () => {
  beforeEach(async () => {
    mockFetch()
    await i18n.changeLanguage("zh-CN")
  })

  it("renders title and tabs", async () => {
    renderConsole()
    await waitFor(() => {
      expect(screen.getByText("集成 / 出站投递")).toBeTruthy()
    })
    expect(screen.getByRole("tab", { name: "概览" })).toBeTruthy()
    expect(screen.getByRole("tab", { name: "Hooks" })).toBeTruthy()
  })

  it("defaults to hooks tab when initialTab=hooks", async () => {
    renderConsole("hooks")
    await waitFor(() => {
      const hooksTab = screen.getByRole("tab", { name: "Hooks" })
      expect(hooksTab.getAttribute("data-state")).toBe("active")
    })
  })

  it("does not show readonly banner for owner with write scopes", async () => {
    renderConsole()
    // 等 me 加载完成（fetch 被调用过）
    await waitFor(() => {
      expect(screen.getByText("集成 / 出站投递")).toBeTruthy()
    })
    // 给 useMe 的 resolve 留一拍
    await waitFor(() => {
      expect(screen.queryByText(/当前身份为只读/)).toBeNull()
    })
  })

  it("shows readonly banner for viewer", async () => {
    vi.spyOn(globalThis, "fetch").mockImplementation((input: unknown) => {
      const url = typeof input === "string" ? input : (input as Request).url
      if (url.includes("/api/v1/credentials/current")) {
        return Promise.resolve(
          new Response(
            JSON.stringify({
              data: {
                actor: { id: "u1", name: "alice" },
                actor_type: "user",
                token: { type: "pat", scopes: ["hook:read"] },
                effective_workspace: { slug: "dajee", name: "dajee" },
                effective_role: "viewer",
              },
            }),
            { status: 200 }
          )
        )
      }
      return Promise.resolve(new Response(JSON.stringify({ data: [] }), { status: 200 }))
    })
    renderConsole()
    await waitFor(() => {
      expect(screen.getByText(/当前身份为只读/)).toBeTruthy()
    })
  })
})

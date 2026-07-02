import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor } from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { i18n } from "../i18n"
import { OverviewPage } from "./OverviewPage"

describe("OverviewPage", () => {
  beforeEach(async () => {
    sessionStorage.clear()
    localStorage.clear()
    await i18n.changeLanguage("zh-CN")
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it("does not render fabricated delivery or audit rows when APIs return empty data", async () => {
    vi.spyOn(globalThis, "fetch").mockImplementation(() =>
      Promise.resolve(new Response(JSON.stringify({ data: [] }), { status: 200 }))
    )
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })

    render(
      <QueryClientProvider client={queryClient}>
        <OverviewPage
          me={{
            actor_type: "user",
            actor: { name: "local" },
            effective_workspace: { slug: "local" },
            effective_role: "owner",
            token: { scopes: ["*"], type: "pat" },
          }}
        />
      </QueryClientProvider>
    )

    await waitFor(() => {
      expect(screen.getAllByText("暂无数据").length).toBeGreaterThan(0)
    })
    expect(screen.queryByText("dead_lettered")).toBeNull()
    expect(screen.queryByText("retry_wait")).toBeNull()
    expect(screen.queryByText("task.done")).toBeNull()
  })
})

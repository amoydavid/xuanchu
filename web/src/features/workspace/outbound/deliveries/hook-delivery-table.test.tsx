import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { ThemeProvider } from "@/components/theme-provider"
import { TooltipProvider } from "@/components/ui/tooltip"
import { i18n } from "@/i18n"

import { HookDeliveryTable } from "./hook-delivery-table"

function renderTable(props: Parameters<typeof HookDeliveryTable>[0]) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={client}>
      <ThemeProvider>
        <TooltipProvider>
          <HookDeliveryTable {...props} />
        </TooltipProvider>
      </ThemeProvider>
    </QueryClientProvider>
  )
}

const deliveriesResponse = {
  data: [
    {
      id: "d1",
      hook_id: "h1",
      event_id: "e1",
      event_type: "task.completed",
      status: "succeeded",
      attempt_count: 1,
      last_status_code: 200,
      last_attempt_at: 1756050000,
      created_at: 1756040000,
      modified_at: 1756050000,
    },
    {
      id: "d2",
      hook_id: "h1",
      event_id: "e2",
      event_type: "task.completed",
      status: "dead_lettered",
      attempt_count: 3,
      last_status_code: 500,
      last_error: "boom",
      last_attempt_at: 1756060000,
      created_at: 1756055000,
      modified_at: 1756060000,
    },
  ],
}

const deliveryDetailResponse = {
  data: {
    ...deliveriesResponse.data[1],
    payload: { event_type: "task.completed", task: { id: "t1" } },
    headers: { "Content-Type": ["application/json"] },
  },
}

function mockFetch() {
  return vi.spyOn(globalThis, "fetch").mockImplementation((input: unknown) => {
    const url = typeof input === "string" ? input : (input as Request).url
    if (url.includes("/api/v1/hook-deliveries/d2")) {
      return Promise.resolve(
        new Response(JSON.stringify(deliveryDetailResponse), { status: 200 })
      )
    }
    if (url.startsWith("/api/v1/hooks/")) {
      return Promise.resolve(
        new Response(JSON.stringify(deliveriesResponse), { status: 200 })
      )
    }
    return Promise.resolve(new Response(JSON.stringify({ data: {} }), { status: 200 }))
  })
}

describe("HookDeliveryTable", () => {
  beforeEach(async () => {
    mockFetch()
    await i18n.changeLanguage("zh-CN")
  })

  it("renders delivery rows with status", async () => {
    renderTable({ hookId: "h1", canWrite: true })
    await waitFor(() => {
      expect(screen.getByText("succeeded")).toBeTruthy()
    })
    expect(screen.getByText("dead_lettered")).toBeTruthy()
  })

  it("shows replay only for failed/dead-letter and write users", async () => {
    renderTable({ hookId: "h1", canWrite: true })
    await waitFor(() => {
      expect(screen.getByText("dead_lettered")).toBeTruthy()
    })
    const replayButtons = screen.getAllByRole("button", { name: /重放/ })
    // 仅 dead_lettered 那一行有 replay
    expect(replayButtons.length).toBe(1)
  })

  it("hides replay for read-only users", async () => {
    renderTable({ hookId: "h1", canWrite: false })
    await waitFor(() => {
      expect(screen.getByText("dead_lettered")).toBeTruthy()
    })
    expect(screen.queryByRole("button", { name: /重放/ })).toBeNull()
  })

  it("opens detail dialog and renders payload JSON", async () => {
    renderTable({ hookId: "h1", canWrite: true })
    await waitFor(() => {
      expect(screen.getByText("dead_lettered")).toBeTruthy()
    })
    const detailButtons = screen.getAllByRole("button", { name: /投递详情/ })
    await userEvent.click(detailButtons[1])
    // 详情 fetch 走 /api/v1/hook-deliveries/d2
    await waitFor(() => {
      expect(vi.mocked(globalThis.fetch)).toHaveBeenCalledWith(
        expect.stringContaining("/api/v1/hook-deliveries/d2"),
        expect.anything()
      )
    })
  })
})

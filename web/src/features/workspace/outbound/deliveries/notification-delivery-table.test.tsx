import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor } from "@testing-library/react"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { ThemeProvider } from "@/components/theme-provider"
import { TooltipProvider } from "@/components/ui/tooltip"
import { i18n } from "@/i18n"

import { NotificationDeliveryTable } from "./notification-delivery-table"

function renderTable(props: Parameters<typeof NotificationDeliveryTable>[0]) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={client}>
      <ThemeProvider>
        <TooltipProvider>
          <NotificationDeliveryTable {...props} />
        </TooltipProvider>
      </ThemeProvider>
    </QueryClientProvider>
  )
}

const deliveriesResponse = {
  data: [
    {
      id: "nd1",
      rule_id: "r1",
      sink_id: "s1",
      task_uuid: "t1",
      object_kind: "task",
      object_id: "t1",
      event_id: "e1",
      event_type: "task.unblocked",
      resolved_url: "https://example.com",
      resolved_endpoint_source: "static_url",
      resolved_endpoint_fingerprint: "sha256:abc",
      rendered_method: "POST",
      rendered_headers: { "Content-Type": ["application/json"] },
      rendered_body: "{}",
      rendered_content_type: "application/json",
      payload: { event_type: "task.unblocked" },
      status: "dead_lettered",
      attempt_count: 3,
      last_status_code: 500,
      last_error: "boom",
      created_at: 1757000000,
      modified_at: 1757000000,
    },
  ],
}

function mockFetch() {
  return vi.spyOn(globalThis, "fetch").mockImplementation((input: unknown) => {
    const url = typeof input === "string" ? input : (input as Request).url
    if (url.startsWith("/api/v1/notification-deliveries")) {
      return Promise.resolve(
        new Response(JSON.stringify(deliveriesResponse), { status: 200 })
      )
    }
    return Promise.resolve(new Response(JSON.stringify({ data: {} }), { status: 200 }))
  })
}

describe("NotificationDeliveryTable", () => {
  beforeEach(async () => {
    mockFetch()
    await i18n.changeLanguage("zh-CN")
  })

  it("lists notification deliveries with event and status", async () => {
    renderTable({ canWrite: true })
    await waitFor(() => {
      expect(screen.getByText("task.unblocked")).toBeTruthy()
    })
    expect(screen.getByText("dead_lettered")).toBeTruthy()
    expect(screen.getByText("boom")).toBeTruthy()
  })

  it("shows replay for dead_lettered when canWrite", async () => {
    renderTable({ canWrite: true })
    await waitFor(() => {
      expect(screen.getByText("dead_lettered")).toBeTruthy()
    })
    expect(screen.getAllByRole("button", { name: /重放/ }).length).toBe(1)
  })

  it("hides replay when read-only", async () => {
    renderTable({ canWrite: false })
    await waitFor(() => {
      expect(screen.getByText("dead_lettered")).toBeTruthy()
    })
    expect(screen.queryByRole("button", { name: /重放/ })).toBeNull()
  })
})

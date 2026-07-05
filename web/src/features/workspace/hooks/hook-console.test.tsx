import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { ThemeProvider } from "@/components/theme-provider"
import { TooltipProvider } from "@/components/ui/tooltip"
import { i18n } from "@/i18n"

import { HookConsole } from "./hook-console"

function renderConsole(canWrite = true) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={client}>
      <ThemeProvider>
        <TooltipProvider>
          <HookConsole canWrite={canWrite} />
        </TooltipProvider>
      </ThemeProvider>
    </QueryClientProvider>
  )
}

const hooksResponse = {
  data: [
    {
      id: "h1",
      name: "task done hook",
      scope_type: "workspace",
      event_types: ["task.done"],
      sink_name: "acme-sink",
      sink_type: "webhook",
      enabled: true,
      timeout_seconds: 10,
      max_attempts: 3,
      created_at: 1756000000,
      modified_at: 1756000000,
    },
    {
      id: "h2",
      name: "project archived",
      scope_type: "workspace",
      event_types: ["project.archived"],
      sink_name: "archive-sink",
      sink_type: "webhook",
      enabled: false,
      timeout_seconds: 10,
      max_attempts: 3,
      created_at: 1756100000,
      modified_at: 1756100000,
    },
  ],
}

const deliveriesResponse = {
  data: [
    {
      id: "d1",
      hook_id: "h1",
      event_id: "e1",
      event_type: "task.done",
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
      event_type: "task.done",
      status: "failed",
      attempt_count: 3,
      last_status_code: 500,
      last_error: "boom",
      last_attempt_at: 1756060000,
      created_at: 1756055000,
      modified_at: 1756060000,
    },
  ],
}

function mockFetch() {
  return vi.spyOn(globalThis, "fetch").mockImplementation((input: unknown) => {
    const url = typeof input === "string" ? input : (input as Request).url
    if (url.startsWith("/api/v1/hooks/h1/deliveries")) {
      return Promise.resolve(new Response(JSON.stringify(deliveriesResponse), { status: 200 }))
    }
    if (url.startsWith("/api/v1/hooks")) {
      return Promise.resolve(new Response(JSON.stringify(hooksResponse), { status: 200 }))
    }
    return Promise.resolve(new Response(JSON.stringify({ data: {} }), { status: 200 }))
  })
}

describe("HookConsole", () => {
  beforeEach(async () => {
    mockFetch()
    await i18n.changeLanguage("zh-CN")
  })

  it("lists hooks with name, events and status", async () => {
    renderConsole()
    await waitFor(() => {
      expect(screen.getByText("task done hook")).toBeTruthy()
    })
    expect(screen.getByText("project archived")).toBeTruthy()
  })

  it("hides write actions when canWrite is false", async () => {
    renderConsole(false)
    await waitFor(() => {
      expect(screen.getByText("task done hook")).toBeTruthy()
    })
    expect(screen.queryByRole("button", { name: /新建/ })).toBeNull()
  })

  it("expands a hook row and loads deliveries", async () => {
    renderConsole()
    await waitFor(() => {
      expect(screen.getByText("task done hook")).toBeTruthy()
    })
    // 点击行展开
    const row = screen.getByText("task done hook")
    await userEvent.click(row)
    await waitFor(() => {
      expect(screen.getByText("succeeded")).toBeTruthy()
    })
    expect(screen.getByText(/boom/)).toBeTruthy()
  })

  it("replays a failed delivery", async () => {
    const fetchMock = mockFetch()
    renderConsole()
    await waitFor(() => {
      expect(screen.getByText("task done hook")).toBeTruthy()
    })
    await userEvent.click(screen.getByText("task done hook"))
    await waitFor(() => {
      expect(screen.getByText(/boom/)).toBeTruthy()
    })
    const replayButtons = screen.getAllByRole("button", { name: /重放/ })
    expect(replayButtons.length).toBeGreaterThan(0)
    await userEvent.click(replayButtons[0])
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        expect.stringContaining("/hook-deliveries/"),
        expect.anything()
      )
    })
  })
})

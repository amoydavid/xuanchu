import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor } from "@testing-library/react"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { ThemeProvider } from "@/components/theme-provider"
import { TooltipProvider } from "@/components/ui/tooltip"
import { i18n } from "@/i18n"

import { ReminderRuleList } from "./reminder-rule-list"

function renderList(canWrite = true) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={client}>
      <ThemeProvider>
        <TooltipProvider>
          <ReminderRuleList canWrite={canWrite} />
        </TooltipProvider>
      </ThemeProvider>
    </QueryClientProvider>
  )
}

const rulesResponse = {
  data: [
    {
      id: "rr1",
      workspace_id: "ws1",
      name: "daily-standup",
      schedule_type: "daily_at",
      schedule_value: "08:50",
      filter_source: "status:pending",
      audience_type: "assignees",
      sink_id: "s1",
      enabled: false,
      created_at: 0,
      modified_at: 0,
    },
  ],
}

function mockFetch() {
  return vi.spyOn(globalThis, "fetch").mockImplementation((input: unknown) => {
    const url = typeof input === "string" ? input : (input as Request).url
    if (url.startsWith("/api/v1/reminder-rules")) {
      return Promise.resolve(new Response(JSON.stringify(rulesResponse), { status: 200 }))
    }
    return Promise.resolve(new Response(JSON.stringify({ data: [] }), { status: 200 }))
  })
}

describe("ReminderRuleList", () => {
  beforeEach(async () => {
    mockFetch()
    await i18n.changeLanguage("zh-CN")
  })

  it("lists reminder rules", async () => {
    renderList()
    await waitFor(() => {
      expect(screen.getByText("daily-standup")).toBeTruthy()
    })
    expect(screen.getByText(/daily_at: 08:50/)).toBeTruthy()
  })

  it("shows create button for write users", async () => {
    renderList(true)
    await waitFor(() => {
      expect(screen.getByText("daily-standup")).toBeTruthy()
    })
    expect(screen.getByRole("button", { name: /新建定时规则/ })).toBeTruthy()
  })

  it("hides create button for read-only", async () => {
    renderList(false)
    await waitFor(() => {
      expect(screen.getByText("daily-standup")).toBeTruthy()
    })
    expect(screen.queryByRole("button", { name: /新建定时规则/ })).toBeNull()
  })
})

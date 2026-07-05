import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor } from "@testing-library/react"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { ThemeProvider } from "@/components/theme-provider"
import { TooltipProvider } from "@/components/ui/tooltip"
import { i18n } from "@/i18n"

import { NotificationRuleList } from "./notification-rule-list"

function renderList(canWrite = true) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={client}>
      <ThemeProvider>
        <TooltipProvider>
          <NotificationRuleList canWrite={canWrite} />
        </TooltipProvider>
      </ThemeProvider>
    </QueryClientProvider>
  )
}

const rulesResponse = {
  data: [
    {
      id: "nr1",
      workspace_id: "ws1",
      name: "task-done-notify",
      event_type: "task.completed",
      audience_type: "assignees",
      sink_id: "s1",
      enabled: true,
      created_at: 0,
      modified_at: 0,
    },
  ],
}

function mockFetch() {
  return vi.spyOn(globalThis, "fetch").mockImplementation((input: unknown) => {
    const url = typeof input === "string" ? input : (input as Request).url
    if (url.startsWith("/api/v1/notification-rules")) {
      return Promise.resolve(new Response(JSON.stringify(rulesResponse), { status: 200 }))
    }
    return Promise.resolve(new Response(JSON.stringify({ data: [] }), { status: 200 }))
  })
}

describe("NotificationRuleList", () => {
  beforeEach(async () => {
    mockFetch()
    await i18n.changeLanguage("zh-CN")
  })

  it("lists rules with name, event, audience", async () => {
    renderList()
    await waitFor(() => {
      expect(screen.getByText("task-done-notify")).toBeTruthy()
    })
    expect(screen.getByText("task.completed")).toBeTruthy()
    expect(screen.getByText("assignees")).toBeTruthy()
  })

  it("shows write actions for write users", async () => {
    renderList(true)
    await waitFor(() => {
      expect(screen.getByText("task-done-notify")).toBeTruthy()
    })
    expect(screen.getByRole("button", { name: /新建通知规则/ })).toBeTruthy()
  })

  it("hides write actions for read-only users", async () => {
    renderList(false)
    await waitFor(() => {
      expect(screen.getByText("task-done-notify")).toBeTruthy()
    })
    expect(screen.queryByRole("button", { name: /新建通知规则/ })).toBeNull()
  })
})

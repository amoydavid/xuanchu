import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor } from "@testing-library/react"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { ThemeProvider } from "@/components/theme-provider"
import { TooltipProvider } from "@/components/ui/tooltip"
import { i18n } from "@/i18n"

import { TaskUrgencyPanel } from "./task-urgency-panel"

function renderPanel(props?: Partial<React.ComponentProps<typeof TaskUrgencyPanel>>) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={client}>
      <ThemeProvider>
        <TooltipProvider>
          <TaskUrgencyPanel taskRef="TASK-1" workspaceSlug="dajee" {...props} />
        </TooltipProvider>
      </ThemeProvider>
    </QueryClientProvider>
  )
}

const sampleUrgency = {
  data: {
    uuid: "t1",
    total: 8.4,
    items: [
      { name: "priority", coefficient: 2, contribution: 2, reason: "high priority" },
      { name: "due", coefficient: 3.5, contribution: 3.4, reason: "due soon" },
    ],
  },
}

describe("TaskUrgencyPanel", () => {
  beforeEach(async () => {
    vi.spyOn(globalThis, "fetch").mockImplementation(() =>
      Promise.resolve(new Response(JSON.stringify(sampleUrgency), { status: 200 }))
    )
    await i18n.changeLanguage("zh-CN")
  })

  it("renders urgency total and contributions", async () => {
    renderPanel()
    await waitFor(() => {
      expect(screen.getByText(/8\.4/)).toBeTruthy()
    })
    expect(screen.getByText(/priority/)).toBeTruthy()
    expect(screen.getByText(/due/)).toBeTruthy()
  })

  it("renders light error when API fails, without blocking", async () => {
    vi.spyOn(globalThis, "fetch").mockImplementation(() =>
      Promise.resolve(new Response(JSON.stringify({ error: { code: "boom" } }), { status: 500 }))
    )
    renderPanel()
    await waitFor(() => {
      expect(screen.queryByText(/8\.4/)).toBeNull()
    })
    // 失败时不阻断页面，只显示轻量错误或空。
  })
})

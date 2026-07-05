import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen } from "@testing-library/react"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { ThemeProvider } from "@/components/theme-provider"
import { TooltipProvider } from "@/components/ui/tooltip"
import { i18n } from "@/i18n"

import { NotificationConsole } from "./notification-console"

describe("NotificationConsole", () => {
  beforeEach(async () => {
    vi.spyOn(globalThis, "fetch").mockImplementation(() =>
      Promise.resolve(new Response(JSON.stringify({ data: [] }), { status: 200 }))
    )
    await i18n.changeLanguage("zh-CN")
  })

  it("uses management wording, not inbox read/delete", () => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    render(
      <QueryClientProvider client={client}>
        <ThemeProvider>
          <TooltipProvider>
            <NotificationConsole />
          </TooltipProvider>
        </ThemeProvider>
      </QueryClientProvider>
    )
    // 文案必须明确「管控」，并显式说明不提供收件箱语义
    expect(screen.getByText(/通知管控/)).toBeTruthy()
    expect(screen.getByText(/不提供/)).toBeTruthy()
  })
})

import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { ThemeProvider } from "@/components/theme-provider"
import { TooltipProvider } from "@/components/ui/tooltip"
import { i18n } from "@/i18n"

import { AppShell } from "./AppShell"

vi.mock("@tanstack/react-router", () => ({
  Link: ({
    children,
    to,
  }: {
    children: React.ReactNode
    to: string
  }) => <a href={to}>{children}</a>,
}))

describe("AppShell", () => {
  beforeEach(async () => {
    await i18n.changeLanguage("zh-CN")
  })

  it("provides a workspace logout action", async () => {
    const onLogout = vi.fn()

    render(
      <ThemeProvider>
        <TooltipProvider>
          <AppShell
            actorName="alice"
            onLogout={onLogout}
            onRefresh={vi.fn()}
            tokenType="pat"
            workspaceSlug="dajee"
          >
            <div>content</div>
          </AppShell>
        </TooltipProvider>
      </ThemeProvider>
    )

    await userEvent.click(screen.getByRole("button", { name: "退出" }))

    expect(onLogout).toHaveBeenCalledTimes(1)
  })
})

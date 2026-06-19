import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { ThemeProvider } from "@/components/theme-provider"
import { TooltipProvider } from "@/components/ui/tooltip"
import { setAdminActingContext, setAdminActingToken } from "@/features/workspace/session/workspace-token"
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
    sessionStorage.clear()
    await i18n.changeLanguage("zh-CN")
  })

  afterEach(() => {
    vi.restoreAllMocks()
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

  it("renders acting banner when acting context exists", () => {
    setAdminActingToken("xuanchu_act_test")
    setAdminActingContext({
      workspaceSlug: "dajee",
      workspaceName: "Dajee",
      actorName: "alice",
      role: "owner",
      adminTokenName: "ops",
    })

    render(
      <ThemeProvider>
        <TooltipProvider>
          <AppShell onLogout={vi.fn()} onRefresh={vi.fn()}>
            <div>content</div>
          </AppShell>
        </TooltipProvider>
      </ThemeProvider>
    )

    // banner 必须展示 workspace、actor、role、admin 来源。
    expect(screen.getByText(/正在以 Dajee 的 alice/)).toBeTruthy()
    expect(screen.getByText(/ops 委托/)).toBeTruthy()
    expect(screen.getByText("返回超管界面")).toBeTruthy()
  })

  it("does not render acting banner in regular workspace console", () => {
    render(
      <ThemeProvider>
        <TooltipProvider>
          <AppShell onLogout={vi.fn()} onRefresh={vi.fn()}>
            <div>content</div>
          </AppShell>
        </TooltipProvider>
      </ThemeProvider>
    )
    expect(screen.queryByText("返回超管界面")).toBeNull()
  })
})

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

  it("replaces logout with acting indicator + return-to-admin in acting mode", () => {
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

    // acting mode：header 显示 actor（alice）+ role，且右上角是「返回超管」而非「退出」。
    expect(screen.getByText(/alice（owner/)).toBeTruthy()
    expect(screen.getByText("返回超管")).toBeTruthy()
    // 不应出现普通退出按钮。
    expect(screen.queryByText("退出")).toBeNull()
  })

  it("shows regular logout button in normal workspace console", () => {
    render(
      <ThemeProvider>
        <TooltipProvider>
          <AppShell onLogout={vi.fn()} onRefresh={vi.fn()}>
            <div>content</div>
          </AppShell>
        </TooltipProvider>
      </ThemeProvider>
    )
    // 普通模式有「退出」，没有「返回超管」。
    expect(screen.getByText("退出")).toBeTruthy()
    expect(screen.queryByText("返回超管")).toBeNull()
  })
})

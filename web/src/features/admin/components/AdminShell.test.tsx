import { render, screen } from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { ThemeProvider } from "@/components/theme-provider"
import { TooltipProvider } from "@/components/ui/tooltip"
import { i18n } from "@/i18n"

import { AdminShell } from "./AdminShell"

vi.mock("@tanstack/react-router", () => ({
  Link: ({
    children,
    className,
    to,
  }: {
    children: React.ReactNode
    className?: string
    to: string
  }) => (
    <a className={className} href={to}>
      {children}
    </a>
  ),
}))

describe("AdminShell", () => {
  beforeEach(async () => {
    await i18n.changeLanguage("zh-CN")
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it("renders admin nav links and token name in header", () => {
    render(
      <ThemeProvider>
        <TooltipProvider>
          <AdminShell onLogout={vi.fn()} onRefresh={vi.fn()} tokenName="ops">
            <div>content</div>
          </AdminShell>
        </TooltipProvider>
      </ThemeProvider>
    )

    expect(screen.getByRole("link", { name: "初始化控制面" })).toBeTruthy()
    expect(screen.getByRole("link", { name: "Workspace 管理" })).toBeTruthy()
    expect(screen.getByRole("link", { name: "Token 管理" })).toBeTruthy()
    // header 标题带 token 名
    expect(screen.getByText(/ops/)).toBeTruthy()
  })

  it("renders mobile hamburger menu button as nav entry", () => {
    render(
      <ThemeProvider>
        <TooltipProvider>
          <AdminShell onLogout={vi.fn()} onRefresh={vi.fn()} tokenName="ops">
            <div>content</div>
          </AdminShell>
        </TooltipProvider>
      </ThemeProvider>
    )

    expect(screen.getByRole("button", { name: "打开菜单" })).toBeTruthy()
    // 桌面端导航仍渲染
    expect(screen.getByRole("link", { name: "Token 管理" })).toBeTruthy()
  })
})

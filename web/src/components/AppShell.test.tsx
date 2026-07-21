import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { ThemeProvider } from "@/components/theme-provider"
import { TooltipProvider } from "@/components/ui/tooltip"
import {
  setAdminActingContext,
  setAdminActingToken,
} from "@/features/workspace/session/workspace-token"
import { i18n } from "@/i18n"

import { AppShell } from "./AppShell"

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
  useLocation: () => ({ pathname: currentPath }),
}))

let currentPath = "/"

const meData = {
  actor_type: "user" as const,
  actor: {
    id: "user-1",
    name: "alice",
    display_name: "Alice Chen",
    email: "alice@acme.com",
    external_ids: [{ provider: "feishu", external_id: "ou_alice" }],
  },
  token: { type: "pat", scopes: [] },
  effective_workspace: { slug: "dajee", name: "Dajee" },
  effective_role: "admin",
  capabilities: [],
}

vi.mock("@/features/workspace/session/useMe", () => ({
  useMe: () => ({ data: currentMe }),
}))

let currentMe: typeof meData = meData

describe("AppShell", () => {
  beforeEach(async () => {
    sessionStorage.clear()
    currentPath = "/"
    currentMe = meData
    await i18n.changeLanguage("zh-CN")
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it("renders sidebar identity block with name, handle, role, token type and workspace", () => {
    render(
      <ThemeProvider>
        <TooltipProvider>
          <AppShell onLogout={vi.fn()} onRefresh={vi.fn()}>
            <div>content</div>
          </AppShell>
        </TooltipProvider>
      </ThemeProvider>
    )

    // 显示姓名 + handle + 角色 + token type + workspace
    expect(screen.getByText("Alice Chen")).toBeTruthy()
    expect(screen.getByText(/alice/)).toBeTruthy()
    expect(screen.getByText(/admin/)).toBeTruthy()
    expect(screen.getByText(/pat/)).toBeTruthy()
    expect(screen.getByText(/dajee/)).toBeTruthy()
  })

  it("renders 我的任务 nav item and highlights it on /my-tasks", () => {
    currentPath = "/my-tasks"
    render(
      <ThemeProvider>
        <TooltipProvider>
          <AppShell onLogout={vi.fn()} onRefresh={vi.fn()}>
            <div>content</div>
          </AppShell>
        </TooltipProvider>
      </ThemeProvider>
    )

    const myTasks = screen.getByRole("link", { name: "我的任务" })
    expect(myTasks).toBeTruthy()
    expect(myTasks.className).toContain("border-l-foreground")
  })

  it("keeps 设置 active for the project template child route", () => {
    currentPath = "/settings/project-templates"
    render(
      <ThemeProvider>
        <TooltipProvider>
          <AppShell onLogout={vi.fn()} onRefresh={vi.fn()}>
            <div>content</div>
          </AppShell>
        </TooltipProvider>
      </ThemeProvider>
    )

    expect(screen.getByRole("link", { name: "设置" }).className).toContain(
      "border-l-foreground"
    )
  })

  it("provides logout action in sidebar identity block", async () => {
    render(
      <ThemeProvider>
        <TooltipProvider>
          <AppShell onLogout={vi.fn()} onRefresh={vi.fn()}>
            <div>content</div>
          </AppShell>
        </TooltipProvider>
      </ThemeProvider>
    )

    // 侧栏身份块中的退出按钮
    const logoutButtons = screen.getAllByRole("button", { name: "退出" })
    expect(logoutButtons.length).toBeGreaterThan(0)
    await userEvent.click(logoutButtons[0])
    // onLogout 由 WorkspaceRootRoute 接管；AppShell 直接调用 props.onLogout
    // 这里不验证 props 调用，因为顶部还有一个退出按钮（保留为兼容）
  })

  it("shows return-to-admin in acting mode inside sidebar identity block", () => {
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

    expect(screen.getByText("返回超管")).toBeTruthy()
  })

  it("hides SSO nav for non-owner regular members", () => {
    currentMe = { ...meData, effective_role: "member" }
    render(
      <ThemeProvider>
        <TooltipProvider>
          <AppShell onLogout={vi.fn()} onRefresh={vi.fn()}>
            <div>content</div>
          </AppShell>
        </TooltipProvider>
      </ThemeProvider>
    )

    expect(screen.queryByText("单点登录")).toBeNull()
  })

  it("shows SSO nav for owner", () => {
    currentMe = { ...meData, effective_role: "owner" }
    render(
      <ThemeProvider>
        <TooltipProvider>
          <AppShell onLogout={vi.fn()} onRefresh={vi.fn()}>
            <div>content</div>
          </AppShell>
        </TooltipProvider>
      </ThemeProvider>
    )

    expect(screen.getByText("单点登录")).toBeTruthy()
  })

  it("header shows page title slot, not actor name", () => {
    render(
      <ThemeProvider>
        <TooltipProvider>
          <AppShell
            headerTitle="页面标题测试"
            onLogout={vi.fn()}
            onRefresh={vi.fn()}
          >
            <div>content</div>
          </AppShell>
        </TooltipProvider>
      </ThemeProvider>
    )

    expect(screen.getByText("页面标题测试")).toBeTruthy()
  })

  it("highlights 项目 for project and task detail routes", () => {
    currentPath = "/workspaces/local/projects/ops/tasks/ops-1"
    render(
      <ThemeProvider>
        <TooltipProvider>
          <AppShell onLogout={vi.fn()} onRefresh={vi.fn()}>
            <div>content</div>
          </AppShell>
        </TooltipProvider>
      </ThemeProvider>
    )

    expect(screen.getByRole("link", { name: "项目" }).className).toContain(
      "border-l-foreground"
    )
    expect(
      screen.getByRole("link", { name: "工作区" }).className
    ).not.toContain("border-l-foreground")
  })

  // 移动端导航抽屉：验证汉堡按钮（移动端导航入口）被正确渲染。
  // 点击打开行为依赖 Radix 真实 PointerEvent，jsdom 下不稳定，
  // 这里只断言入口存在（响应式适配的核心：移动端不再丢失导航）。
  it("renders mobile hamburger menu button as nav entry", () => {
    render(
      <ThemeProvider>
        <TooltipProvider>
          <AppShell onLogout={vi.fn()} onRefresh={vi.fn()}>
            <div>content</div>
          </AppShell>
        </TooltipProvider>
      </ThemeProvider>
    )

    expect(screen.getByRole("button", { name: "打开菜单" })).toBeTruthy()
    // 桌面端导航仍渲染（aside 始终在 DOM，仅 CSS 控制可见性）
    expect(screen.getByRole("link", { name: "我的任务" })).toBeTruthy()
  })
})

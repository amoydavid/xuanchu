import { render, screen, waitFor } from "@testing-library/react"
import { beforeEach, describe, expect, it, vi } from "vitest"
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  Outlet,
  RouterProvider,
} from "@tanstack/react-router"

import { ThemeProvider } from "@/components/theme-provider"
import { TooltipProvider } from "@/components/ui/tooltip"
import { AdminShell } from "@/features/admin/components/AdminShell"
import { i18n } from "@/i18n"

import { BootstrapWizard } from "./bootstrap-wizard"

// AdminShell 的 <Link> 需要 Router context。测试里用最小 memory router 包裹。
function wrapWithRouter(ui: React.ReactElement) {
  const rootRoute = createRootRoute({
    component: () => <Outlet />,
  })
  const indexRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/admin",
    component: () => ui,
  })
  const routeTree = rootRoute.addChildren([indexRoute])
  const router = createRouter({
    routeTree,
    history: createMemoryHistory({ initialEntries: ["/admin"] }),
  })
  return <RouterProvider router={router} />
}

describe("admin console i18n", () => {
  beforeEach(async () => {
    localStorage.clear()
    sessionStorage.clear()
    await i18n.changeLanguage("zh-CN")
  })

  it("renders server admin shell and bootstrap wizard in Chinese", async () => {
    render(
      wrapWithRouter(
        <ThemeProvider>
          <TooltipProvider>
            <AdminShell
              onLogout={vi.fn()}
              onRefresh={vi.fn()}
              tokenName="local-admin"
            >
              <BootstrapWizard />
            </AdminShell>
          </TooltipProvider>
        </ThemeProvider>
      )
    )

    // RouterProvider 异步加载，等 shell 标题渲染后再断言
    await waitFor(() => {
      expect(screen.getByText("服务端超管 · local-admin")).toBeTruthy()
    })
    expect(
      screen.getByRole("navigation", { name: "服务端超管导航" })
    ).toBeTruthy()
    expect(
      screen.getByRole("link", { name: "初始化控制面" }).getAttribute("href")
    ).toBe("/admin")
    expect(screen.getAllByText("高危操作").length).toBeGreaterThan(0)
    expect(screen.getByText("创建工作区")).toBeTruthy()
    expect(screen.getByText("工作区管理员")).toBeTruthy()
    expect(screen.getByText("管理员 Agent token")).toBeTruthy()
    expect(screen.getByText("创建工作区和管理员")).toBeTruthy()
    expect(screen.getByLabelText("工作区 slug")).toBeTruthy()
    expect(screen.getByLabelText("管理员姓名")).toBeTruthy()
    expect(screen.getByLabelText("Token 名称")).toBeTruthy()

    expect(screen.queryByText("Server Admin · local-admin")).toBeNull()
    expect(screen.queryByText("High risk")).toBeNull()
    expect(screen.queryByText("Create workspace and admin")).toBeNull()
  })
})

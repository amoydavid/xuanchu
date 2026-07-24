import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { ThemeProvider } from "@/components/theme-provider"
import { TooltipProvider } from "@/components/ui/tooltip"
import {
  setAdminActingContext,
  setAdminActingToken,
  setWorkspaceToken,
} from "@/features/workspace/session/workspace-token"
import { i18n } from "@/i18n"
import { renderWithRouter } from "@/test/router-wrapper"

import { WorkspaceRootRoute } from "./WorkspaceRootRoute"

// 整页跳转（SSO 登出分支会用）。mock 模块以 spy 调用。
const navigateSpy = vi.fn()
vi.mock("@/lib/browser-navigation", () => ({
  navigateToDocument: (path: string) => navigateSpy(path),
}))

function renderRoute() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const result = render(
    renderWithRouter(
      <QueryClientProvider client={queryClient}>
        <ThemeProvider>
          <TooltipProvider>
            <WorkspaceRootRoute />
          </TooltipProvider>
        </ThemeProvider>
      </QueryClientProvider>
    )
  )
  return { result, queryClient }
}

function credentialsCurrentResponse(
  overrides: Record<string, unknown> = {}
) {
  return Promise.resolve(
    new Response(
      JSON.stringify({
        data: {
          actor_type: "user",
          actor: {
            id: "u1",
            name: "alice",
            display_name: "Alice",
            email: "alice@example.com",
          },
          token: {
            id: "act-1",
            name: "admin-acting-ops",
            type: "admin_acting",
            scopes: ["task:read"],
          },
          visible_workspaces: [{ id: "ws-1", slug: "dajee", name: "Dajee" }],
          effective_workspace: { id: "ws-1", slug: "dajee", name: "Dajee" },
          effective_role: "owner",
          capabilities: ["task:read"],
          ...overrides,
        },
      }),
      { status: 200 }
    )
  )
}

describe("WorkspaceRootRoute acting mode", () => {
  beforeEach(async () => {
    sessionStorage.clear()
    await i18n.changeLanguage("zh-CN")
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it("treats acting token as signed-in and does not show login page", async () => {
    // 只设置 acting token + context，不设置普通 workspace token。
    setAdminActingToken("xuanchu_act_test")
    setAdminActingContext({
      workspaceSlug: "dajee",
      workspaceName: "Dajee",
      actorName: "alice",
      role: "owner",
      adminTokenName: "ops",
    })
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockImplementation(() => credentialsCurrentResponse())

    renderRoute()

    // 不应该出现登录页文案；应该渲染出 AppShell（账户切换器触发器常驻显示工作空间名）。
    await waitFor(() => {
      expect(screen.getByRole("button", { name: /Dajee/ })).toBeTruthy()
    })
    expect(screen.queryByText(/登录凭证仅保存在/)).toBeNull()
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/v1/credentials/current",
      expect.anything()
    )

    fetchMock.mockRestore()
  })

  it("shows login page when neither workspace nor acting token exists", async () => {
    renderRoute()

    await waitFor(() => {
      expect(screen.queryByText(/登录凭证仅保存在/)).toBeTruthy()
    })
  })

  it("treats normal workspace token as signed-in (regression)", async () => {
    setWorkspaceToken("xuanchu_pat_test")
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockImplementation(() => credentialsCurrentResponse())

    renderRoute()

    await waitFor(() => {
      expect(screen.getByRole("button", { name: /Dajee/ })).toBeTruthy()
    })

    fetchMock.mockRestore()
  })

  it("shows tenant system identity from credentials current", async () => {
    setWorkspaceToken("xuanchu_tenant_test")
    vi.spyOn(globalThis, "fetch").mockImplementation(() =>
      credentialsCurrentResponse({
        actor_type: "tenant_access_token",
        actor: {
          id: "tok-1",
          name: "runtime-prod",
          display_name: "系统身份 / runtime-prod",
        },
        token: {
          id: "tok-1",
          name: "runtime-prod",
          type: "tenant_access_token",
          scopes: ["task:read"],
        },
        capabilities: ["task:read"],
      })
    )

    renderRoute()

    // AppShell 已渲染（signedIn）；系统身份在账户切换浮层内，展开后校验。
    const trigger = await screen.findByRole("button", { name: /Dajee/ })
    await userEvent.click(trigger)
    await waitFor(() => {
      expect(screen.getByText(/系统身份 \/ runtime-prod/)).toBeTruthy()
    })
  })
})

describe("WorkspaceRootRoute SSO logout", () => {
  beforeEach(async () => {
    sessionStorage.clear()
    // 模拟 SSO cookie 模式：xuanchu_csrf 由后端 SSO 登录写入（非 HttpOnly）。
    document.cookie = "xuanchu_csrf=abc; path=/"
    navigateSpy.mockClear()
    await i18n.changeLanguage("zh-CN")
  })

  afterEach(() => {
    vi.restoreAllMocks()
    // 清掉 cookie（jsdom 无法精确删除，覆盖为过期）。
    document.cookie = "xuanchu_csrf=; path=/; max-age=0"
  })

  it("SSO 模式点退出会调后端 /auth/logout（带 CSRF header）并整页跳转（回归）", async () => {
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockImplementation((input: URL | RequestInfo) => {
        const url = typeof input === "string" ? input : String(input)
        // credentials/current 由 useMe 拉取，返回正常用户
        if (url.includes("/api/v1/credentials/current")) {
          return credentialsCurrentResponse()
        }
        // logout 端点返回 200（redirect:"manual" 下 fetch 拿到的是 opaque 响应，
        // 这里给个 200 即可，调用方不读 body）
        if (url.includes("/auth/logout")) {
          return Promise.resolve(new Response("", { status: 200 }))
        }
        return Promise.resolve(new Response("{}", { status: 200 }))
      })

    renderRoute()

    // 等 AppShell 渲染出账户切换器（说明已判定 signedIn=true）
    await waitFor(() => {
      expect(screen.getByRole("button", { name: /Dajee/ })).toBeTruthy()
    })

    // 打开账户切换浮层，点击退出
    await userEvent.click(screen.getByRole("button", { name: /Dajee/ }))
    const logoutItem = await screen.findByRole("menuitem", { name: "退出" })
    await userEvent.click(logoutItem)

    // 等待 async onLogout 完成
    await waitFor(() => {
      expect(navigateSpy).toHaveBeenCalledWith("/")
    })

    // 验证确实发起了 logout，且带正确的 CSRF header（double-submit）
    const logoutCall = fetchMock.mock.calls.find(([url]) =>
      String(url).includes("/auth/logout")
    )
    expect(logoutCall).toBeTruthy()
    const init = logoutCall?.[1] as RequestInit | undefined
    expect(init?.method).toBe("POST")
    expect((init?.headers as Record<string, string>)["X-Xuanchu-CSRF"]).toBe(
      "abc"
    )
    // redirect: manual，避免跟随 302
    expect(init?.redirect).toBe("manual")

    fetchMock.mockRestore()
  })
})

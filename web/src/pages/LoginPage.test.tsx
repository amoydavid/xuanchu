import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { i18n } from "../i18n"
import { ThemeProvider } from "@/components/theme-provider"
import { TooltipProvider } from "@/components/ui/tooltip"
import { renderWithRouter } from "@/test/router-wrapper"
import { LoginPage } from "./LoginPage"

function okResponse(data: unknown) {
  return Promise.resolve(new Response(JSON.stringify({ data }), { status: 200 }))
}

describe("LoginPage", () => {
  beforeEach(async () => {
    localStorage.clear()
    sessionStorage.clear()
    await i18n.changeLanguage("zh-CN")
  })

  it("renders the sign-in heading and supports language switching", async () => {
    const onSignedIn = vi.fn()
    render(
      renderWithRouter(
        <ThemeProvider>
          <TooltipProvider>
            <LoginPage onSignedIn={onSignedIn} />
          </TooltipProvider>
        </ThemeProvider>
      )
    )
    await waitFor(() => {
      expect(screen.getByText("璇础")).toBeTruthy()
    })
    // 标题简化为「登录」（signInTitle 不再带品牌名插值）
    expect(screen.getByRole("heading", { name: "登录" })).toBeTruthy()
    expect(screen.getByRole("button", { name: "登录" })).toBeTruthy()
    // 无 SSO 时凭证输入直接展开，label 为「登录凭证」
    expect(screen.getByLabelText("登录凭证")).toBeTruthy()
    await userEvent.click(screen.getByRole("combobox", { name: "语言" }))
    await userEvent.click(screen.getByRole("option", { name: "English" }))
    expect(screen.getByText("Xuanchu")).toBeTruthy()
    expect(screen.getByRole("button", { name: "Sign in" })).toBeTruthy()
  })

  it("no longer exposes the standalone admin login link", async () => {
    render(
      renderWithRouter(
        <ThemeProvider>
          <TooltipProvider>
            <LoginPage onSignedIn={vi.fn()} />
          </TooltipProvider>
        </ThemeProvider>
      )
    )

    await waitFor(() => {
      expect(screen.getByRole("heading", { name: "登录" })).toBeTruthy()
    })
    // 超管入口已并入凭证输入（按前缀分流），不再有独立链接
    expect(screen.queryByText("服务端超管登录 ->")).toBeNull()
    expect(screen.queryByRole("tab")).toBeNull()
    expect(screen.queryByRole("radio")).toBeNull()
  })

  it("shows the protected page that will open after sign-in", async () => {
    render(
      renderWithRouter(
        <ThemeProvider>
          <TooltipProvider>
            <LoginPage
              onSignedIn={vi.fn()}
              redirectPath="/workspaces/acme/projects/agentapi"
            />
          </TooltipProvider>
        </ThemeProvider>
      )
    )

    await waitFor(() => {
      expect(screen.getByText("登录后继续访问：")).toBeTruthy()
    })
    expect(
      screen.getByText("/workspaces/acme/projects/agentapi")
    ).toBeTruthy()
    expect(
      screen.queryByText("https://xuanchu.example.com/workspaces/acme")
    ).toBeNull()
  })

  it("validates tenant token with credentials current", async () => {
    const onSignedIn = vi.fn()
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockImplementation(() =>
        okResponse({
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
          effective_workspace: { id: "ws-1", slug: "dajee", name: "Dajee" },
          effective_role: "owner",
          capabilities: ["task:read"],
        })
      )

    render(
      renderWithRouter(
        <ThemeProvider>
          <TooltipProvider>
            <LoginPage onSignedIn={onSignedIn} />
          </TooltipProvider>
        </ThemeProvider>
      )
    )

    await userEvent.type(
      await screen.findByLabelText("登录凭证"),
      "xuanchu_tenant_test"
    )
    await userEvent.click(screen.getByRole("button", { name: "登录" }))

    await waitFor(() => {
      expect(onSignedIn).toHaveBeenCalledTimes(1)
    })
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/v1/credentials/current",
      expect.objectContaining({
        headers: expect.objectContaining({
          Authorization: "Bearer xuanchu_tenant_test",
        }),
      })
    )
    expect(sessionStorage.getItem("xuanchu.console.token")).toBe(
      "xuanchu_tenant_test"
    )
  })

  it("shows specific message for token_web_login_disabled error", async () => {
    vi.spyOn(globalThis, "fetch").mockImplementation(() =>
      Promise.resolve(
        new Response(
          JSON.stringify({ error: { code: "token_web_login_disabled" } }),
          { status: 403 }
        )
      )
    )

    render(
      renderWithRouter(
        <ThemeProvider>
          <TooltipProvider>
            <LoginPage onSignedIn={vi.fn()} />
          </TooltipProvider>
        </ThemeProvider>
      )
    )

    await userEvent.type(
      await screen.findByLabelText("登录凭证"),
      "xuanchu_pat_sso"
    )
    await userEvent.click(screen.getByRole("button", { name: "登录" }))

    await waitFor(() => {
      expect(
        screen.getByText(/不能用于登录 Web Console/)
      ).toBeTruthy()
    })
  })

  it("routes admin-prefixed credential to admin session and storage", async () => {
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockImplementation(() =>
        okResponse({
          actor_type: "admin",
          actor: { id: "admin-1", name: "root" },
        })
      )

    // jsdom 29 的 window.location 锁定，整体替换为可写 stub 记录 href 跳转。
    const originalLocation = window.location
    const hrefSetter = { href: "https://localhost/" }
    Object.defineProperty(window, "location", {
      configurable: true,
      value: {
        ...originalLocation,
        get href() {
          return hrefSetter.href
        },
        set href(v: string) {
          hrefSetter.href = v
        },
      },
    })

    render(
      renderWithRouter(
        <ThemeProvider>
          <TooltipProvider>
            <LoginPage onSignedIn={vi.fn()} />
          </TooltipProvider>
        </ThemeProvider>
      )
    )

    await userEvent.type(
      await screen.findByLabelText("登录凭证"),
      "xuanchu_admin_test"
    )
    await userEvent.click(screen.getByRole("button", { name: "登录" }))

    await waitFor(() => {
      expect(hrefSetter.href).toContain("/admin")
    })
    // 超管 token 写入 admin 存储 key，不污染 workspace token key
    expect(sessionStorage.getItem("xuanchu.console.admin_token")).toBe(
      "xuanchu_admin_test"
    )
    expect(sessionStorage.getItem("xuanchu.console.token")).toBeNull()
    // 走的是 admin 校验端点，不是 workspace 的 credentials/current
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/v1/admin/session",
      expect.objectContaining({
        headers: expect.objectContaining({
          Authorization: "Bearer xuanchu_admin_test",
        }),
      })
    )

    Object.defineProperty(window, "location", {
      configurable: true,
      value: originalLocation,
    })
  })
})

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

  it("validates token and supports language switching", async () => {
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
    expect(
      screen.getByRole("heading", { name: "使用璇础访问凭证登录" })
    ).toBeTruthy()
    expect(document.querySelector(".mb-5 .size-14")).toBeTruthy()
    expect(screen.getByRole("button", { name: "登录" })).toBeTruthy()
    await userEvent.click(screen.getByRole("combobox", { name: "语言" }))
    await userEvent.click(screen.getByRole("option", { name: "English" }))
    expect(screen.getByText("Xuanchu")).toBeTruthy()
    expect(screen.getByRole("button", { name: "Sign in" })).toBeTruthy()
  })

  it("shows an admin login link without mode switching", async () => {
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
      expect(
        screen.getByRole("link", { name: "服务端超管登录 ->" })
      ).toBeTruthy()
    })
    expect(
      screen
        .getByRole("link", { name: "服务端超管登录 ->" })
        .getAttribute("href")
    ).toBe("/admin/login")
    expect(screen.queryByRole("tab")).toBeNull()
    expect(screen.queryByRole("radio")).toBeNull()
    expect(sessionStorage.getItem("xuanchu.console.admin_token")).toBeNull()
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
      await screen.findByLabelText("Token"),
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
})

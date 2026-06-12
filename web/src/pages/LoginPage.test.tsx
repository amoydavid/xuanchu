import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { i18n } from "../i18n"
import { ThemeProvider } from "@/components/theme-provider"
import { TooltipProvider } from "@/components/ui/tooltip"
import { LoginPage } from "./LoginPage"

describe("LoginPage", () => {
  beforeEach(async () => {
    localStorage.clear()
    sessionStorage.clear()
    await i18n.changeLanguage("zh-CN")
  })

  it("validates token and supports language switching", async () => {
    const onSignedIn = vi.fn()
    render(
      <ThemeProvider>
        <TooltipProvider>
          <LoginPage onSignedIn={onSignedIn} />
        </TooltipProvider>
      </ThemeProvider>
    )
    expect(screen.getByText("璇础")).toBeTruthy()
    expect(screen.getByRole("heading", { name: "使用璇础 token 登录" })).toBeTruthy()
    expect(document.querySelector(".mb-5 .size-14")).toBeTruthy()
    expect(screen.getByRole("button", { name: "登录" })).toBeTruthy()
    await userEvent.click(screen.getByRole("combobox", { name: "语言" }))
    await userEvent.click(screen.getByRole("option", { name: "English" }))
    expect(screen.getByText("Xuanchu")).toBeTruthy()
    expect(screen.getByRole("button", { name: "Sign in" })).toBeTruthy()
  })

  it("shows an admin login link without mode switching", () => {
    render(
      <ThemeProvider>
        <TooltipProvider>
          <LoginPage onSignedIn={vi.fn()} />
        </TooltipProvider>
      </ThemeProvider>
    )

    expect(
      screen
        .getByRole("link", { name: "服务端超管登录 ->" })
        .getAttribute("href")
    ).toBe("/admin/login")
    expect(screen.queryByRole("tab")).toBeNull()
    expect(screen.queryByRole("radio")).toBeNull()
    expect(sessionStorage.getItem("xuanchu.console.admin_token")).toBeNull()
  })

  it("shows the protected page that will open after sign-in", () => {
    render(
      <ThemeProvider>
        <TooltipProvider>
          <LoginPage
            onSignedIn={vi.fn()}
            redirectPath="/workspaces/acme/projects/agentapi"
          />
        </TooltipProvider>
      </ThemeProvider>
    )

    expect(screen.getByText("登录后继续访问：")).toBeTruthy()
    expect(
      screen.getByText("/workspaces/acme/projects/agentapi")
    ).toBeTruthy()
    expect(
      screen.queryByText("https://xuanchu.example.com/workspaces/acme")
    ).toBeNull()
  })
})

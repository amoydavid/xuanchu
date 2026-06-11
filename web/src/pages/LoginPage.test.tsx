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
    expect(screen.getByRole("button", { name: "登录" })).toBeTruthy()
    await userEvent.click(screen.getByRole("combobox", { name: "语言" }))
    await userEvent.click(screen.getByRole("option", { name: "English" }))
    expect(screen.getByRole("button", { name: "Sign in" })).toBeTruthy()
  })
})

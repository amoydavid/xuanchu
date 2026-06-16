import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { ThemeProvider } from "@/components/theme-provider"
import { TooltipProvider } from "@/components/ui/tooltip"
import { i18n } from "@/i18n"
import { renderWithRouter } from "@/test/router-wrapper"

import { AdminSetupPage } from "./AdminSetupPage"

describe("AdminSetupPage", () => {
  beforeEach(async () => {
    localStorage.clear()
    sessionStorage.clear()
    await i18n.changeLanguage("en-US")
  })

  it("creates the first admin token without storing it as a browser session", async () => {
    const fetchMock = vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response(
        JSON.stringify({
          data: {
            token: "xuanchu_admin_created",
            token_name: "primary",
            token_prefix: "xuanchu_admin_cr",
          },
        }),
        { status: 201 }
      )
    )

    render(
      renderWithRouter(
        <ThemeProvider>
          <TooltipProvider>
            <AdminSetupPage />
          </TooltipProvider>
        </ThemeProvider>
      )
    )

    await waitFor(() => {
      expect(
        screen.getByRole("heading", { name: "Create server admin token" })
      ).toBeTruthy()
    })
    await userEvent.type(screen.getByLabelText("Setup code"), "setup-code")
    await userEvent.clear(screen.getByLabelText("Token name"))
    await userEvent.type(screen.getByLabelText("Token name"), "primary")
    await userEvent.click(
      screen.getByRole("button", { name: "Create admin token" })
    )

    expect(fetchMock.mock.calls[0]?.[0]).toBe("/api/v1/admin/setup")
    expect(fetchMock.mock.calls[0]?.[1]).toMatchObject({
      method: "POST",
      body: JSON.stringify({
        setup_code: "setup-code",
        name: "primary",
      }),
    })
    expect(await screen.findByText("xuanchu_admin_created")).toBeTruthy()
    expect(sessionStorage.getItem("xuanchu.console.admin_token")).toBeNull()
  })
})

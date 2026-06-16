import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { ThemeProvider } from "@/components/theme-provider"
import { TooltipProvider } from "@/components/ui/tooltip"
import { i18n } from "@/i18n"
import { renderWithRouter } from "@/test/router-wrapper"

import { AdminLoginPage } from "./AdminLoginPage"

describe("AdminLoginPage", () => {
  beforeEach(async () => {
    vi.restoreAllMocks()
    localStorage.clear()
    sessionStorage.clear()
    await i18n.changeLanguage("en-US")
  })

  it("uses admin session validation and admin token storage", async () => {
    const onSignedIn = vi.fn()
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            data: { status: "login_required", setup_required: false },
          }),
          { status: 200 }
        )
      )
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({ data: { token_name: "ops", capabilities: [] } }),
          {
            status: 200,
          }
        )
      )

    render(
      renderWithRouter(
        <ThemeProvider>
          <TooltipProvider>
            <AdminLoginPage onSignedIn={onSignedIn} />
          </TooltipProvider>
        </ThemeProvider>
      )
    )

    await waitFor(() => {
      expect(
        screen.getByRole("heading", { name: "Server Admin Control Plane" })
      ).toBeTruthy()
    })
    expect(screen.getByText("High risk")).toBeTruthy()
    await userEvent.type(
      screen.getByLabelText("Admin token"),
      "xuanchu_admin_test"
    )
    await userEvent.click(screen.getByRole("button", { name: "Authenticate" }))

    expect(fetchMock.mock.calls[0]?.[0]).toBe("/api/v1/admin/status")
    expect(fetchMock.mock.calls[1]?.[0]).toBe("/api/v1/admin/session")
    expect(sessionStorage.getItem("xuanchu.console.admin_token")).toBe(
      "xuanchu_admin_test"
    )
    expect(sessionStorage.getItem("xuanchu.console.token")).toBeNull()
    expect(onSignedIn).toHaveBeenCalled()
  })

  it("shows a generic admin auth error", async () => {
    vi.spyOn(globalThis, "fetch")
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            data: { status: "login_required", setup_required: false },
          }),
          { status: 200 }
        )
      )
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({ error: { code: "admin_auth_invalid" } }),
          {
            status: 401,
          }
        )
      )

    render(
      renderWithRouter(
        <ThemeProvider>
          <TooltipProvider>
            <AdminLoginPage onSignedIn={vi.fn()} />
          </TooltipProvider>
        </ThemeProvider>
      )
    )

    await waitFor(() => {
      expect(screen.getByLabelText("Admin token")).toBeTruthy()
    })
    await userEvent.type(screen.getByLabelText("Admin token"), "bad")
    await userEvent.click(screen.getByRole("button", { name: "Authenticate" }))
    expect(await screen.findByText("Admin authentication failed")).toBeTruthy()
  })

  it("links to setup when the server has no valid admin token", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response(
        JSON.stringify({
          data: { status: "setup_required", setup_required: true },
        }),
        { status: 200 }
      )
    )

    render(
      renderWithRouter(
        <ThemeProvider>
          <TooltipProvider>
            <AdminLoginPage onSignedIn={vi.fn()} />
          </TooltipProvider>
        </ThemeProvider>
      )
    )

    const link = await screen.findByRole("link", {
      name: "Open admin setup",
    })
    expect(link.getAttribute("href")).toBe("/admin/setup")
    expect(screen.queryByLabelText("Admin token")).toBeNull()
  })
})

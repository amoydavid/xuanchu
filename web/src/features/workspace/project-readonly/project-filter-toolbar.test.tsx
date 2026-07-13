import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { i18n } from "@/i18n"

import { ProjectFilterToolbar } from "./project-filter-toolbar"

const navigateMock = vi.fn()

vi.mock("@tanstack/react-router", () => ({
  useNavigate: () => navigateMock,
}))

describe("ProjectFilterToolbar", () => {
  beforeEach(async () => {
    navigateMock.mockReset()
    await i18n.changeLanguage("en-US")
  })

  it("commits text filters on blur instead of every keystroke", async () => {
    render(
      <ProjectFilterToolbar
        filter={{}}
        toParams={{ projectSlug: "agentapi", workspaceSlug: "acme" }}
      />
    )

    const assignee = screen.getByPlaceholderText("Assignee")
    await userEvent.type(assignee, "alice")

    expect(navigateMock).not.toHaveBeenCalled()

    await userEvent.tab()

    expect(navigateMock).toHaveBeenCalledTimes(1)
  })

  it("resets local text input when the filter value changes outside the input", async () => {
    const { rerender } = render(
      <ProjectFilterToolbar
        filter={{ assignee: "alice" }}
        toParams={{ projectSlug: "agentapi", workspaceSlug: "acme" }}
      />
    )

    expect(
      screen.getByPlaceholderText<HTMLInputElement>("Assignee").value
    ).toBe("alice")

    rerender(
      <ProjectFilterToolbar
        filter={{}}
        toParams={{ projectSlug: "agentapi", workspaceSlug: "acme" }}
      />
    )

    expect(
      screen.getByPlaceholderText<HTMLInputElement>("Assignee").value
    ).toBe("")
  })

  it("uses shadcn buttons for active filter chips", () => {
    render(
      <ProjectFilterToolbar
        filter={{ q: "needle", status: "pending" }}
        toParams={{ projectSlug: "agentapi", workspaceSlug: "acme" }}
      />
    )

    expect(
      screen
        .getByRole("button", { name: "Status=Pending" })
        .getAttribute("data-slot")
    ).toBe("button")
    expect(
      screen
        .getByRole("button", { name: "Clear all" })
        .getAttribute("data-slot")
    ).toBe("button")
  })

  it("does not expose the retired recurring task status", async () => {
    render(
      <ProjectFilterToolbar
        filter={{}}
        toParams={{ projectSlug: "agentapi", workspaceSlug: "acme" }}
      />
    )

    await userEvent.click(screen.getAllByRole("combobox")[0])
    expect(screen.queryByRole("option", { name: "Recurring" })).toBeNull()
  })
})

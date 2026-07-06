import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import type { ReactNode } from "react"
import { describe, expect, it, vi } from "vitest"

import "@/i18n"
import { workspaceApiGet } from "@/features/workspace/session/workspace-api"

import { TokenForm, valuesToCreateInput } from "./token-form"
import type { TokenFormValues } from "./token-api"

vi.mock("@/features/workspace/session/workspace-api", () => ({
  workspaceApiGet: vi.fn(),
}))

function makeWrapper(queryClient: QueryClient) {
  return function Wrapper({ children }: { children: ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    )
  }
}

function makeQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  })
}

describe("TokenForm", () => {
  it("uses shadcn checkbox controls for workspace selection", async () => {
    vi.mocked(workspaceApiGet).mockResolvedValue([
      {
        archived_at: null,
        id: "w1",
        name: "Acme",
        slug: "acme",
        visibility: "team",
      },
    ])
    const onSubmit = vi.fn()

    render(<TokenForm mode="create" onSubmit={onSubmit} submitting={false} />, {
      wrapper: makeWrapper(makeQueryClient()),
    })

    await screen.findByText("Acme")
    const checkbox = screen.getByRole("checkbox", { name: "Acme" })

    expect(checkbox.getAttribute("data-slot")).toBe("checkbox")
    const submitButton = screen.getByRole("button", {
      name: /Create Token|创建/,
    })
    expect(submitButton.getAttribute("data-slot")).toBe("button")
    await userEvent.click(checkbox)

    expect(checkbox.getAttribute("aria-checked")).toBe("true")
  })

  describe("valuesToCreateInput user field", () => {
    const base: TokenFormValues = {
      name: "ci",
      type: "pat",
      user: "",
      workspaces: [],
      scopes: ["task:read"],
      projects: [],
      expiresPreset: "never",
      expiresAt: "",
    }

    it("omits user when empty (token belongs to actor)", () => {
      const input = valuesToCreateInput(base)
      expect("user" in input).toBe(false)
    })

    it("includes user when admin picks a target member", () => {
      const input = valuesToCreateInput({ ...base, user: "u-zhang" })
      expect(input.user).toBe("u-zhang")
    })
  })
})

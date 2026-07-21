import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import type { ReactNode } from "react"
import { afterEach, beforeEach, expect, it, vi } from "vitest"

import { ThemeProvider } from "@/components/theme-provider"
import { TooltipProvider } from "@/components/ui/tooltip"
import { setWorkspaceToken } from "@/features/workspace/session/workspace-token"
import { i18n } from "@/i18n"

import { ProjectTemplatesRoute } from "./ProjectTemplatesRoute"

vi.mock("@tanstack/react-router", () => ({
  Link: ({ children, to, ...props }: { children: ReactNode; to: string }) => (
    <a href={to} {...props}>
      {children}
    </a>
  ),
}))

vi.mock("@/features/workspace/session/useMe", () => ({
  useMe: () => ({
    data: {
      effective_workspace: { slug: "acme" },
      effective_role: "owner",
      token: {
        scopes: ["project:write", "task:write", "config:write", "hook:write"],
      },
    },
  }),
}))

function response(data: unknown) {
  return Promise.resolve(
    new Response(JSON.stringify({ data }), {
      headers: { "Content-Type": "application/json" },
      status: 200,
    })
  )
}

beforeEach(async () => {
  sessionStorage.clear()
  setWorkspaceToken("xuanchu_pat_test")
  await i18n.changeLanguage("zh-CN")
})

afterEach(() => {
  vi.restoreAllMocks()
  sessionStorage.clear()
})

it("starts the create capture wizard from the real route empty CTA", async () => {
  vi.spyOn(globalThis, "fetch").mockImplementation((input) => {
    const url = String(input)
    if (url === "/api/v1/projects?workspace=acme&status=all") {
      return response([
        {
          id: "project-1",
          workspace_id: "workspace-1",
          slug: "launch-source",
          name: "上线来源项目",
          status: "active",
          task_count: 0,
          pending_count: 0,
          completed_count: 0,
          created_at: 1,
          modified_at: 1,
        },
      ])
    }
    if (url.endsWith("/resolve-selection?workspace=acme")) {
      return response({ refs: [], total: 0, source_hash: "source-hash" })
    }
    return response({ items: [], total: 0, limit: 20, offset: 0 })
  })
  const queryClient = new QueryClient({
    defaultOptions: { mutations: { retry: false }, queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={queryClient}>
      <ThemeProvider>
        <TooltipProvider>
          <ProjectTemplatesRoute />
        </TooltipProvider>
      </ThemeProvider>
    </QueryClientProvider>
  )

  await userEvent.click(
    await screen.findByRole("button", { name: "保存新模板" })
  )
  expect(screen.getByRole("dialog", { name: "选择来源项目" })).toBeTruthy()
  await userEvent.click(screen.getByRole("button", { name: /上线来源项目/ }))
  expect(
    await screen.findByRole("dialog", { name: "保存项目模板" })
  ).toBeTruthy()
})

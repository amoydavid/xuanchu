import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, waitFor } from "@testing-library/react"
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  Outlet,
  RouterProvider,
} from "@tanstack/react-router"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { i18n } from "@/i18n"
import { ProjectSettingsNotesRoute } from "./ProjectSettingsNotesRoute"

// 用临时 router 测试 settings/notes 重定向：源路由 + 目标路由都要注册，
// Navigate 才能解析目标 path。
function renderRedirectRouter(initialPath: string) {
  const rootRoute = createRootRoute({
    component: () => (
      <QueryClientProvider client={new QueryClient()}>
        <Outlet />
      </QueryClientProvider>
    ),
  })
  const notesRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/projects/$projectSlug/settings/notes",
    component: ProjectSettingsNotesRoute,
  })
  const activityRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/workspaces/$workspaceSlug/projects/$projectSlug/activity",
    component: () => <div data-testid="activity-target">activity</div>,
  })
  const routeTree = rootRoute.addChildren([notesRoute, activityRoute])
  const router = createRouter({
    routeTree,
    history: createMemoryHistory({ initialEntries: [initialPath] }),
  })
  render(<RouterProvider router={router} />)
  return router
}

describe("ProjectSettingsNotesRoute redirect", () => {
  beforeEach(async () => {
    vi.clearAllMocks()
    await i18n.changeLanguage("zh-CN")
    vi.spyOn(globalThis, "fetch").mockImplementation((input: URL | RequestInfo) => {
      const url = typeof input === "string" ? input : String(input)
      if (url.includes("/api/v1/credentials/current")) {
        return Promise.resolve(
          new Response(
            JSON.stringify({
              data: {
                actor_type: "user",
                actor: { id: "u1", name: "alice" },
                token: { type: "pat", scopes: ["project:read"] },
                effective_workspace: { slug: "local" },
                effective_role: "owner",
              },
            }),
            { status: 200 }
          )
        )
      }
      return Promise.resolve(new Response("{}", { status: 200 }))
    })
  })

  it("redirects settings/notes to project activity", async () => {
    const router = renderRedirectRouter("/projects/ops/settings/notes")
    await waitFor(() => {
      expect(router.state.location.pathname).toBe(
        "/workspaces/local/projects/ops/activity"
      )
    })
  })
})

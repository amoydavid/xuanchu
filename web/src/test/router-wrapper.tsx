import { createMemoryHistory, createRootRoute, createRoute, createRouter, Outlet, RouterProvider } from "@tanstack/react-router"
import type React from "react"

// 测试用最小 router context，供含 <Link> 的组件在无真实 router 时渲染。
// 默认初始路径 /，可传 initialEntries 指定。
export function renderWithRouter(
  ui: React.ReactElement,
  initialEntries: string[] = ["/"]
) {
  const rootRoute = createRootRoute({
    component: () => <Outlet />,
  })
  const indexRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/",
    component: () => ui,
  })
  const routeTree = rootRoute.addChildren([indexRoute])
  const router = createRouter({
    routeTree,
    history: createMemoryHistory({ initialEntries }),
  })
  return <RouterProvider router={router} />
}

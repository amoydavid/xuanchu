import {
  Outlet,
  createRootRoute,
  createRoute,
  createRouter,
} from "@tanstack/react-router"
import { Suspense, lazy, type ComponentType } from "react"

import type { PageKey } from "@/components/AppShell"

const WorkspaceRootRoute = lazy(() =>
  import("@/routes/workspace/WorkspaceRootRoute").then((module) => ({
    default: module.WorkspaceRootRoute,
  }))
)
const OverviewRoute = lazy(() =>
  import("@/routes/workspace/OverviewRoute").then((module) => ({
    default: module.OverviewRoute,
  }))
)
const ResourceRoute = lazy(() =>
  import("@/routes/workspace/ResourceRoute").then((module) => ({
    default: module.ResourceRoute,
  }))
)
const ProjectReadonlyRoute = lazy(() =>
  import("@/routes/workspace/ProjectReadonlyRoute").then((module) => ({
    default: module.ProjectReadonlyRoute,
  }))
)
const ProjectTaskDetailRoute = lazy(() =>
  import("@/routes/workspace/ProjectTaskDetailRoute").then((module) => ({
    default: module.ProjectTaskDetailRoute,
  }))
)
const AdminLoginRoute = lazy(() =>
  import("@/routes/admin/AdminLoginRoute").then((module) => ({
    default: module.AdminLoginRoute,
  }))
)
const AdminSetupRoute = lazy(() =>
  import("@/routes/admin/AdminSetupRoute").then((module) => ({
    default: module.AdminSetupRoute,
  }))
)
const AdminGuardRoute = lazy(() =>
  import("@/routes/admin/AdminGuardRoute").then((module) => ({
    default: module.AdminGuardRoute,
  }))
)
const AdminDashboardRoute = lazy(() =>
  import("@/routes/admin/AdminDashboardRoute").then((module) => ({
    default: module.AdminDashboardRoute,
  }))
)

const rootRoute = createRootRoute({
  component: () => <Outlet />,
})

const workspaceRootRoute = createRoute({
  getParentRoute: () => rootRoute,
  id: "workspace",
  component: lazyRoute(WorkspaceRootRoute),
})

const indexRoute = createRoute({
  getParentRoute: () => workspaceRootRoute,
  path: "/",
  component: lazyRoute(OverviewRoute),
})

const adminLoginRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/admin/login",
  component: lazyRoute(AdminLoginRoute),
})

const adminSetupRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/admin/setup",
  component: lazyRoute(AdminSetupRoute),
})

const adminGuardRoute = createRoute({
  getParentRoute: () => rootRoute,
  id: "admin",
  component: lazyRoute(AdminGuardRoute),
})

const adminDashboardRoute = createRoute({
  getParentRoute: () => adminGuardRoute,
  path: "/admin",
  component: lazyRoute(AdminDashboardRoute),
})

const projectReadonlyRoute = createRoute({
  getParentRoute: () => workspaceRootRoute,
  path: "/workspaces/$workspaceSlug/projects/$projectSlug",
  component: lazyRoute(ProjectReadonlyRoute),
})

const projectTaskDetailRoute = createRoute({
  getParentRoute: () => workspaceRootRoute,
  path: "/workspaces/$workspaceSlug/projects/$projectSlug/tasks/$taskRef",
  component: lazyRoute(ProjectTaskDetailRoute),
})

const TokensRoute = lazy(() =>
  import("@/routes/workspace/TokensRoute").then((module) => ({
    default: module.TokensRoute,
  }))
)
const tokensRoute = createRoute({
  getParentRoute: () => workspaceRootRoute,
  path: "/tokens",
  component: lazyRoute(TokensRoute),
})

const routeTree = rootRoute.addChildren([
  workspaceRootRoute.addChildren([
    indexRoute,
    createResourceRoute("tasks", "/tasks"),
    createResourceRoute("projects", "/projects"),
    createResourceRoute("workspaces", "/workspaces"),
    createResourceRoute("members", "/members"),
    tokensRoute,
    createResourceRoute("hooks", "/hooks"),
    createResourceRoute("notifications", "/notifications"),
    createResourceRoute("audit", "/audit"),
    createResourceRoute("settings", "/settings"),
    projectReadonlyRoute,
    projectTaskDetailRoute,
  ]),
  adminLoginRoute,
  adminSetupRoute,
  adminGuardRoute.addChildren([adminDashboardRoute]),
])

export const router = createRouter({ routeTree })

declare module "@tanstack/react-router" {
  interface Register {
    router: typeof router
  }
}

function createResourceRoute(page: PageKey, path: string) {
  return createRoute({
    getParentRoute: () => workspaceRootRoute,
    path,
    component: () => (
      <Suspense fallback={<RouteFallback />}>
        <ResourceRoute page={page} />
      </Suspense>
    ),
  })
}

function lazyRoute(Component: ComponentType) {
  return function LazyRoute() {
    return (
      <Suspense fallback={<RouteFallback />}>
        <Component />
      </Suspense>
    )
  }
}

function RouteFallback() {
  return (
    <div
      aria-label="Loading"
      className="h-24 animate-pulse border bg-card"
      role="status"
    />
  )
}

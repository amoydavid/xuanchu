import {
  Outlet,
  createRootRoute,
  createRoute,
  createRouter,
  redirect,
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
const ProjectsListRoute = lazy(() =>
  import("@/routes/workspace/ProjectsListRoute").then((module) => ({
    default: module.ProjectsListRoute,
  }))
)
const MembersRoute = lazy(() =>
  import("@/routes/workspace/MembersRoute").then((module) => ({
    default: module.MembersRoute,
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
const SsoLoginRouteComponent = lazy(() =>
  import("@/pages/SsoLoginPage").then((module) => ({
    default: module.SsoLoginPage,
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

const AdminTokensRoute = lazy(() =>
  import("@/routes/admin/AdminTokensRoute").then((module) => ({
    default: module.AdminTokensRoute,
  }))
)

const AdminWorkspacesRoute = lazy(() =>
  import("@/routes/admin/AdminWorkspacesRoute").then((module) => ({
    default: module.AdminWorkspacesRoute,
  }))
)

const AdminWorkspaceDetailRoute = lazy(() =>
  import("@/routes/admin/AdminWorkspaceDetailRoute").then((module) => ({
    default: module.AdminWorkspaceDetailRoute,
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
const ssoLoginRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/sso/login/$slug",
  component: lazyRoute(SsoLoginRouteComponent),
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

const adminTokensRoute = createRoute({
  getParentRoute: () => adminGuardRoute,
  path: "/admin/tokens",
  component: lazyRoute(AdminTokensRoute),
})

const adminWorkspacesRoute = createRoute({
  getParentRoute: () => adminGuardRoute,
  path: "/admin/workspaces",
  component: lazyRoute(AdminWorkspacesRoute),
})

const adminWorkspaceDetailRoute = createRoute({
  getParentRoute: () => adminGuardRoute,
  path: "/admin/workspaces/$workspaceSlug",
  component: lazyRoute(AdminWorkspaceDetailRoute),
})

const projectReadonlyRoute = createRoute({
  getParentRoute: () => workspaceRootRoute,
  path: "/workspaces/$workspaceSlug/projects/$projectSlug",
  component: lazyRoute(ProjectReadonlyRoute),
  validateSearch: (search: Record<string, unknown>): Record<string, string> => {
    const out: Record<string, string> = {}
    for (const key of [
      "status",
      "priority",
      "assignee",
      "due_after",
      "due_before",
      "tags",
      "q",
    ]) {
      const value = search[key]
      if (typeof value === "string" && value !== "") {
        out[key] = value
      }
    }
    return out
  },
})

const projectsListRoute = createRoute({
  getParentRoute: () => workspaceRootRoute,
  path: "/projects",
  component: lazyRoute(ProjectsListRoute),
})

const membersRoute = createRoute({
  getParentRoute: () => workspaceRootRoute,
  path: "/members",
  component: lazyRoute(MembersRoute),
})

// /tasks 已下线：任务统一从项目入口浏览，旧链接重定向到 /projects。
const tasksRedirectRoute = createRoute({
  getParentRoute: () => workspaceRootRoute,
  path: "/tasks",
  beforeLoad: () => {
    throw redirect({ to: "/projects" })
  },
  component: () => null,
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

const SsoRoute = lazy(() =>
  import("@/routes/workspace/SsoRoute").then((module) => ({
    default: module.SsoRoute,
  }))
)
const ssoRoute = createRoute({
  getParentRoute: () => workspaceRootRoute,
  path: "/sso",
  component: lazyRoute(SsoRoute),
})

const routeTree = rootRoute.addChildren([
  workspaceRootRoute.addChildren([
    indexRoute,
    tasksRedirectRoute,
    projectsListRoute,
    createResourceRoute("workspaces", "/workspaces"),
    membersRoute,
    tokensRoute,
    ssoRoute,
    createResourceRoute("hooks", "/hooks"),
    createResourceRoute("notifications", "/notifications"),
    createResourceRoute("audit", "/audit"),
    createResourceRoute("settings", "/settings"),
    projectReadonlyRoute,
    projectTaskDetailRoute,
  ]),
  adminLoginRoute,
  ssoLoginRoute,
  adminSetupRoute,
  adminGuardRoute.addChildren([
    adminDashboardRoute,
    adminTokensRoute,
    adminWorkspacesRoute,
    adminWorkspaceDetailRoute,
  ]),
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

import { useState } from "react"
import { useQuery, useQueryClient } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"

import { AppShell, type PageKey } from "@/components/AppShell"
import { OverviewPage } from "@/pages/OverviewPage"
import { LoginPage } from "@/pages/LoginPage"
import { ResourcePage, statusCell, textCell, userCell } from "@/pages/ResourcePage"
import { apiGet } from "@/lib/api"
import { getToken } from "@/lib/token"

type MeResponse = {
  actor: { name: string }
  token: { type: string; scopes: string[] }
  effective_workspace: { slug: string }
}

function App() {
  const [signedIn, setSignedIn] = useState(() => getToken() !== null)
  const [activePage, setActivePage] = useState<PageKey>("overview")
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const me = useQuery({
    enabled: signedIn,
    queryKey: ["me"],
    queryFn: () => apiGet<MeResponse>("/api/v1/me"),
  })

  if (!signedIn) {
    return <LoginPage onSignedIn={() => setSignedIn(true)} />
  }

  const workspaceSlug = me.data?.effective_workspace.slug

  return (
    <AppShell
      activePage={activePage}
      onNavigate={setActivePage}
      onRefresh={() => void queryClient.invalidateQueries()}
      actorName={me.data?.actor.name}
      tokenType={me.data?.token.type}
      workspaceSlug={workspaceSlug}
    >
      {activePage === "overview" ? (
        <OverviewPage me={me.data} />
      ) : (
        <ResourcePage
          {...resourceConfig(activePage, t, workspaceSlug)}
        />
      )}
    </AppShell>
  )
}

function resourceConfig(
  page: PageKey,
  t: (key: string) => string,
  workspaceSlug?: string
): React.ComponentProps<typeof ResourcePage> {
  switch (page) {
    case "tasks":
      return {
        title: t("page.tasks"),
        path: "/api/v1/tasks?limit=50",
        columns: [
          { key: "description", header: t("common.name"), render: textCell("description") },
          { key: "status", header: t("common.status"), render: statusCell("status") },
          { key: "project", header: t("overview.projects"), render: textCell("project") },
        ],
      }
    case "projects":
      return {
        title: t("page.projects"),
        path: "/api/v1/projects",
        columns: [
          { key: "slug", header: t("resource.slug"), render: textCell("slug") },
          { key: "name", header: t("common.name"), render: textCell("name") },
          { key: "archived", header: t("resource.archived"), render: statusCell("archived") },
        ],
      }
    case "workspaces":
      return {
        title: t("page.workspaces"),
        path: "/api/v1/workspaces",
        columns: [
          { key: "slug", header: t("resource.slug"), render: textCell("slug") },
          { key: "name", header: t("common.name"), render: textCell("name") },
          { key: "role", header: t("resource.role"), render: statusCell("role") },
        ],
      }
    case "members":
      return {
        enabled: Boolean(workspaceSlug),
        title: t("page.members"),
        path: `/api/v1/workspaces/${workspaceSlug ?? ""}/members`,
        columns: [
          { key: "user", header: t("common.actor"), render: userCell("user") },
          { key: "role", header: t("resource.role"), render: statusCell("role") },
        ],
      }
    case "tokens":
      return {
        title: t("page.tokens"),
        path: "/api/v1/tokens",
        columns: [
          { key: "name", header: t("common.name"), render: textCell("name") },
          { key: "type", header: t("resource.type"), render: statusCell("type") },
          { key: "prefix", header: t("resource.prefix"), render: textCell("prefix") },
        ],
      }
    case "hooks":
      return {
        title: t("page.hooks"),
        path: "/api/v1/hooks",
        columns: [
          { key: "name", header: t("common.name"), render: textCell("name") },
          { key: "scope_type", header: t("overview.scope"), render: statusCell("scope_type") },
          { key: "enabled", header: t("resource.enabled"), render: statusCell("enabled") },
        ],
      }
    case "notifications":
      return {
        title: t("page.notifications"),
        path: "/api/v1/notification-sinks",
        columns: [
          { key: "name", header: t("common.name"), render: textCell("name") },
          { key: "type", header: t("resource.type"), render: statusCell("type") },
          { key: "enabled", header: t("resource.enabled"), render: statusCell("enabled") },
        ],
      }
    case "audit":
      return {
        title: t("page.audit"),
        path: "/api/v1/audit?limit=50",
        columns: [
          { key: "action", header: t("overview.action"), render: textCell("action") },
          { key: "actor", header: t("common.actor"), render: userCell("actor") },
          { key: "target_type", header: t("overview.target"), render: textCell("target_type") },
        ],
      }
    case "settings":
    case "overview":
      return {
        title: t(`page.${page}`),
        path: "/api/v1/config",
        columns: [
          { key: "key", header: t("resource.key"), render: textCell("key") },
          { key: "value", header: t("resource.value"), render: textCell("value") },
        ],
      }
  }
}

export default App

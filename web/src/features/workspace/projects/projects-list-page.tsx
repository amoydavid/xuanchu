import { useNavigate } from "@tanstack/react-router"
import { useQuery } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"

import { DataTable, type Column } from "@/components/DataTable"
import { Badge } from "@/components/ui/badge"
import { Skeleton } from "@/components/ui/skeleton"

import { getProjects, type ProjectSummary } from "./projects-api"

type ProjectsListPageProps = {
  workspaceSlug: string
}

export function ProjectsListPage({ workspaceSlug }: ProjectsListPageProps) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { data: projects, isPending, isError } = useQuery({
    queryKey: ["projects", workspaceSlug],
    queryFn: () => getProjects(true),
  })

  if (isPending) {
    return (
      <div className="space-y-4 p-6">
        <Skeleton className="h-7 w-32" />
        <Skeleton className="h-40 w-full" />
      </div>
    )
  }
  if (isError) {
    return <div className="p-6 text-destructive">{t("common.error")}</div>
  }

  const columns: Column<ProjectSummary>[] = [
    {
      key: "name",
      header: t("projects.colName"),
      render: (p) => <span className="font-medium">{p.name || p.slug}</span>,
    },
    {
      key: "status",
      header: t("common.status"),
      render: (p) => <Badge variant="outline">{p.status}</Badge>,
    },
    {
      key: "progress",
      header: t("projects.colProgress"),
      render: (p) => <ProgressBadge taskCount={p.task_count} completedCount={p.completed_count} />,
    },
    {
      key: "task_count",
      header: t("projects.colTaskCount"),
      render: (p) => (
        <span className="text-muted-foreground">
          {p.pending_count} / {p.task_count}
        </span>
      ),
    },
  ]

  return (
    <div className="space-y-4 p-6">
      <h1 className="text-xl font-semibold">{t("projects.title")}</h1>
      <DataTable
        rows={projects}
        columns={columns}
        empty={t("projects.empty")}
        onRowClick={(p) =>
          navigate({
            to: "/workspaces/$workspaceSlug/projects/$projectSlug",
            params: { workspaceSlug, projectSlug: p.slug },
          })
        }
      />
      <p className="text-xs text-muted-foreground">{t("projects.colTaskCountHint")}</p>
    </div>
  )
}

// ProgressBadge 渲染完成进度条 + 百分比。task_count 为 0 时显示空占位。
function ProgressBadge({ taskCount, completedCount }: { taskCount: number; completedCount: number }) {
  if (taskCount === 0) {
    return <span className="text-muted-foreground">-</span>
  }
  const pct = Math.round((completedCount / taskCount) * 100)
  return (
    <div className="flex items-center gap-2">
      <div className="h-1.5 w-16 overflow-hidden rounded-full bg-muted">
        <div className="h-full bg-primary" style={{ width: `${pct}%` }} />
      </div>
      <span className="text-xs text-muted-foreground">{pct}%</span>
    </div>
  )
}

import { useState } from "react"
import { BoxesIcon, PlusIcon } from "lucide-react"
import { useNavigate } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"

import { DataTable, type Column } from "@/components/DataTable"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { taskStatusLabel } from "@/features/workspace/shared/task-labels"
import { useMe } from "@/features/workspace/session/useMe"
import { ProjectTemplateInstantiateWizard } from "@/features/workspace/project-templates/instantiate/project-template-instantiate-wizard"
import { canProjectManage } from "@/features/workspace/project-workbench/permissions/permissions"

import type { ProjectWorkbenchProject } from "../api/project-api"
import { useProjectsQuery } from "../hooks/use-project-data"
import { ProjectCreateDialog } from "./project-create-dialog"
import { ProjectRowActions } from "./project-row-actions"

type ProjectsListPageProps = {
  workspaceSlug: string
}

export function ProjectsListPage({ workspaceSlug }: ProjectsListPageProps) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const me = useMe()
  const writeScopes = me.data?.token?.scopes
  const canManage = canProjectManage({
    role: me.data?.effective_role,
    scopes: writeScopes,
  })
  const [createOpen, setCreateOpen] = useState(false)
  const [instantiateOpen, setInstantiateOpen] = useState(false)
  const { data: projects = [], isPending, isError } = useProjectsQuery(
    workspaceSlug,
    "all"
  )
  function openProject(projectSlug: string) {
    void navigate({
      to: "/workspaces/$workspaceSlug/projects/$projectSlug",
      params: { workspaceSlug, projectSlug },
    })
  }

  if (isPending) {
    return (
      <div className="space-y-4 p-6">
        <div className="flex items-center justify-between">
          <Skeleton className="h-8 w-40" />
          <Skeleton className="h-8 w-24" />
        </div>
        <Skeleton className="h-48 w-full" />
      </div>
    )
  }

  if (isError) {
    return <div className="p-6 text-destructive">{t("common.error")}</div>
  }

  const columns: Column<ProjectWorkbenchProject>[] = [
    {
      key: "name",
      header: t("projectWorkbench.projects.column.project"),
      render: (project) => (
        <div className="min-w-0">
          <div className="font-medium">{project.name || project.slug}</div>
          <div className="text-xs text-muted-foreground">{project.slug}</div>
        </div>
      ),
    },
    {
      key: "status",
      header: t("common.status"),
      render: (project) => (
        <Badge variant="outline">{taskStatusLabel(project.status, t)}</Badge>
      ),
    },
    {
      key: "progress",
      header: t("projectWorkbench.projects.column.progress"),
      render: (project) => (
        <ProjectProgress
          taskCount={project.task_count}
          completedCount={project.completed_count}
        />
      ),
    },
    {
      key: "tasks",
      header: t("projectWorkbench.projects.column.tasks"),
      render: (project) => (
        <span className="text-muted-foreground">
          {project.pending_count} / {project.task_count}
        </span>
      ),
    },
    {
      key: "actions",
      header: t("common.actions"),
      render: (project) => (
        <ProjectRowActions
          canManage={canManage}
          onOpen={openProject}
          projectSlug={project.slug}
          status={project.status}
          workspaceSlug={workspaceSlug}
        />
      ),
    },
  ]

  return (
    <div className="space-y-4 p-6">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h1 className="text-xl font-semibold">
            {t("projectWorkbench.projects.title")}
          </h1>
          <p className="mt-1 text-sm text-muted-foreground">
            {t("projectWorkbench.projects.subtitle")}
          </p>
        </div>
        <div className="flex flex-wrap gap-2">
          {canManage ? (
            <Button onClick={() => setInstantiateOpen(true)} variant="outline">
              <BoxesIcon data-icon="inline-start" />
              从模板创建
            </Button>
          ) : null}
          <Button onClick={() => setCreateOpen(true)}>
            <PlusIcon data-icon="inline-start" />
            {t("projectWorkbench.projects.create.button")}
          </Button>
        </div>
      </div>

      <DataTable
        rows={projects}
        columns={columns}
        empty={t("projectWorkbench.projects.empty")}
        onRowClick={(project) => openProject(project.slug)}
      />

      <ProjectCreateDialog
        open={createOpen}
        onOpenChange={setCreateOpen}
        workspaceSlug={workspaceSlug}
        onCreated={(project) => {
          setCreateOpen(false)
          openProject(project.slug)
        }}
      />
      <ProjectTemplateInstantiateWizard
        canInstantiate={canManage}
        canManage={canManage}
        onOpenChange={setInstantiateOpen}
        open={instantiateOpen}
        writeScopes={writeScopes}
        workspaceSlug={workspaceSlug}
      />
    </div>
  )
}

function ProjectProgress({
  completedCount,
  taskCount,
}: {
  completedCount: number
  taskCount: number
}) {
  if (taskCount === 0) {
    return <span className="text-muted-foreground">-</span>
  }

  const pct = Math.round((completedCount / taskCount) * 100)
  return (
    <div className="flex items-center gap-2">
      <div className="h-1.5 w-20 overflow-hidden rounded-full bg-muted">
        <div className="h-full bg-primary" style={{ width: `${pct}%` }} />
      </div>
      <span className="w-9 text-xs text-muted-foreground">{pct}%</span>
    </div>
  )
}

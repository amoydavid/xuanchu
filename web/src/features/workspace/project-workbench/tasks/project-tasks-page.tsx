import { useMemo, useState } from "react"
import { useQuery } from "@tanstack/react-query"
import { useNavigate, useSearch } from "@tanstack/react-router"
import { PlusIcon, UploadIcon } from "lucide-react"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import {
  filterToTaskQuery,
  type TaskFilter,
} from "@/features/workspace/project-readonly/project-filter"
import { useMe } from "@/features/workspace/session/useMe"
import { getWorkspaceMembers } from "../api/users-api"
import { useProjectTasksQuery } from "../hooks/use-project-data"
import { canTaskWrite } from "../permissions/permissions"
import { TaskImportDialog } from "../import/task-import-dialog"
import { useProjectLayout } from "../project/project-layout"
import { TaskCreateDialog } from "./task-create-dialog"
import { ProjectTaskToolbar } from "./project-task-toolbar"
import { TaskTable } from "./task-table"

type ProjectTasksPageProps = {
  projectSlug: string
  workspaceSlug: string
}

// ProjectTasksPage 是项目执行页：任务筛选、简单任务列表、新建、导入、行内编辑。
// 第一阶段只做简单列表，不做分组、父子树、看板或泳道。
// 新建/导入是 Tasks 页主动作，不出现在共享项目 Header 中。
export function ProjectTasksPage({
  projectSlug,
  workspaceSlug,
}: ProjectTasksPageProps) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const [importOpen, setImportOpen] = useState(false)
  const [createOpen, setCreateOpen] = useState(false)
  const search = useSearch({ strict: false }) as Partial<TaskFilter>
  const filter: TaskFilter = useMemo(
    () => ({
      status: typeof search.status === "string" ? search.status : undefined,
      priority:
        typeof search.priority === "string" ? search.priority : undefined,
      assignee:
        typeof search.assignee === "string" ? search.assignee : undefined,
      due_after:
        typeof search.due_after === "string" ? search.due_after : undefined,
      due_before:
        typeof search.due_before === "string" ? search.due_before : undefined,
      due_empty:
        typeof search.due_empty === "string" ? search.due_empty : undefined,
      assignee_empty:
        typeof search.assignee_empty === "string"
          ? search.assignee_empty
          : undefined,
      wait_before:
        typeof search.wait_before === "string" ? search.wait_before : undefined,
      scheduled_before:
        typeof search.scheduled_before === "string"
          ? search.scheduled_before
          : undefined,
      until_before:
        typeof search.until_before === "string"
          ? search.until_before
          : undefined,
      tags: typeof search.tags === "string" ? search.tags : undefined,
      q: typeof search.q === "string" ? search.q : undefined,
      query: typeof search.query === "string" ? search.query : undefined,
      sort: typeof search.sort === "string" ? search.sort : undefined,
    }),
    [
      search.assignee,
      search.assignee_empty,
      search.due_after,
      search.due_before,
      search.due_empty,
      search.priority,
      search.q,
      search.query,
      search.scheduled_before,
      search.sort,
      search.status,
      search.tags,
      search.until_before,
      search.wait_before,
    ]
  )
  const filterQuery = useMemo(() => filterToTaskQuery(filter), [filter])
  const me = useMe()
  const canCreateTask = canTaskWrite({
    role: me.data?.effective_role,
    scopes: me.data?.token.scopes,
  })
  const { closed } = useProjectLayout()
  const tasks = useProjectTasksQuery(workspaceSlug, projectSlug, filterQuery)
  const members = useQuery({
    queryKey: ["workspace-members", workspaceSlug],
    queryFn: () => getWorkspaceMembers(workspaceSlug),
  })

  const taskRows = useMemo(() => tasks.data ?? [], [tasks.data])
  const assigneeOptions = useMemo(
    () =>
      (members.data ?? []).map((member) => ({
        email: member.email,
        id: member.user_id,
        label: member.display_name || member.name || member.email || member.user_id,
        name: member.name,
      })),
    [members.data]
  )

  const canEditTasks = canCreateTask && !closed

  const setTaskFilters = (
    values: Partial<Record<keyof TaskFilter, string>>
  ) => {
    void navigate({
      to: "/workspaces/$workspaceSlug/projects/$projectSlug/tasks",
      params: { workspaceSlug, projectSlug },
      search: (prev) => {
        const next = { ...(prev as Record<string, string>) }
        for (const [key, value] of Object.entries(values)) {
          if (value) {
            next[key] = value
          } else {
            delete next[key]
          }
        }
        return next
      },
    })
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-end gap-2">
        {canEditTasks ? (
          <>
            <Button
              aria-label={t("projectSubpages.taskImportLabel")}
              onClick={() => setImportOpen(true)}
              size="icon"
              title={t("projectSubpages.taskImportLabel")}
              type="button"
              variant="ghost"
            >
              <UploadIcon className="h-4 w-4" />
            </Button>
            <Button
              aria-label={t("projectSubpages.taskCreateLabel")}
              onClick={() => setCreateOpen(true)}
              size="icon"
              title={t("projectSubpages.taskCreateLabel")}
              type="button"
              variant="ghost"
            >
              <PlusIcon className="h-4 w-4" />
            </Button>
          </>
        ) : null}
      </div>
      <ProjectTaskToolbar
        assigneeOptions={assigneeOptions}
        canCreateTask={canEditTasks}
        filter={filter}
        navigateTo="/workspaces/$workspaceSlug/projects/$projectSlug/tasks"
        onCreateTask={() => setCreateOpen(true)}
        toParams={{ workspaceSlug, projectSlug }}
      />
      <TaskTable
        canWrite={canEditTasks}
        onSortChange={(sort) => setTaskFilters({ sort })}
        projectSlug={projectSlug}
        sort={filter.sort}
        tasks={taskRows}
        workspaceSlug={workspaceSlug}
      />
      <TaskImportDialog
        existingTasks={taskRows}
        onOpenChange={setImportOpen}
        open={importOpen}
        projectSlug={projectSlug}
        workspaceSlug={workspaceSlug}
      />
      <TaskCreateDialog
        filters={filterQuery}
        onOpenChange={setCreateOpen}
        open={createOpen}
        projectSlug={projectSlug}
        workspaceSlug={workspaceSlug}
      />
    </div>
  )
}

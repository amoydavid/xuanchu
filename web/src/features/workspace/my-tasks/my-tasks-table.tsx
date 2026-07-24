import { Link } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"

import { Badge } from "@/components/ui/badge"
import { Checkbox } from "@/components/ui/checkbox"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { taskStatusLabel } from "@/features/workspace/shared/task-labels"
import { EditFeedbackProvider } from "@/features/workspace/project-workbench/shared/edit-feedback"
import type { ProjectWorkbenchTask } from "@/features/workspace/project-workbench/api/project-api"
import {
  taskDisplayRef,
  taskRouteRef,
} from "@/features/workspace/project-workbench/tasks/task-reference"
import { TaskRowActions } from "@/features/workspace/project-workbench/tasks/task-row-actions"
import { saveMyTasksReturnState } from "./my-tasks-return-state"
import { MyTasksBulkActions } from "./my-tasks-bulk-actions"

type MyTasksTableProps = {
  canWrite?: boolean
  onSelectedIdsChange?: (ids: string[]) => void
  returnSearch?: string
  selectedIds?: string[]
  sort?: string
  tasks: ProjectWorkbenchTask[]
  workspaceSlug: string
  onSortChange?: (sort: string) => void
}

const priorityLabel: Record<string, string> = {
  H: "高",
  M: "中",
  L: "低",
}

export function MyTasksTable({
  canWrite = false,
  onSelectedIdsChange = () => {},
  onSortChange,
  returnSearch = "",
  selectedIds = [],
  sort,
  tasks,
  workspaceSlug,
}: MyTasksTableProps) {
  const { t } = useTranslation()
  if (tasks.length === 0) {
    return (
      <section className="rounded-lg border bg-card p-6 text-sm text-muted-foreground">
        {t("myTasks.empty")}
      </section>
    )
  }

  const selectableTasks = canWrite
    ? tasks.filter((task) => task.status !== "deleted" && taskKey(task))
    : []
  const selectedSet = new Set(selectedIds)
  const selectedTasks = tasks.filter((task) => selectedSet.has(taskKey(task)))
  const allSelected =
    selectableTasks.length > 0 &&
    selectableTasks.every((task) => selectedSet.has(taskKey(task)))
  const someSelected = selectedTasks.length > 0 && !allSelected
  const toggleTask = (task: ProjectWorkbenchTask, checked: boolean) => {
    const id = taskKey(task)
    if (!id) return
    onSelectedIdsChange(
      checked
        ? [...new Set([...selectedIds, id])]
        : selectedIds.filter((item) => item !== id)
    )
  }
  const removeSelected = (ids: string[]) => {
    const removed = new Set(ids)
    onSelectedIdsChange(selectedIds.filter((id) => !removed.has(id)))
  }

  return (
    <EditFeedbackProvider>
      <section className="space-y-2">
      <MyTasksBulkActions
        onRemoveSucceeded={removeSelected}
        selectedTasks={selectedTasks}
        workspaceSlug={workspaceSlug}
      />
        <Table containerClassName="hidden md:block">
          <TableHeader>
            <TableRow>
              {canWrite ? (
                <TableHead className="w-10">
                  <Checkbox
                    aria-label={t("myTasks.bulk.selectAll")}
                    checked={allSelected ? true : someSelected ? "indeterminate" : false}
                    onCheckedChange={(checked) =>
                      onSelectedIdsChange(
                        checked ? selectableTasks.map(taskKey) : []
                      )
                    }
                  />
                </TableHead>
              ) : null}
              <TableHead>{t("projectReadonly.identifier")}</TableHead>
              <TableHead>{t("projectReadonly.title")}</TableHead>
              <TableHead>{t("common.status")}</TableHead>
              <TableHead>{t("projectReadonly.priority")}</TableHead>
              <SortHead
                active={sort === "due"}
                label={t("projectReadonly.due")}
                onSortChange={onSortChange}
                sort="due"
              />
              <TableHead>{t("projectReadonly.project")}</TableHead>
              <TableHead className="text-right">{t("common.actions")}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {tasks.map((task) => (
              <MyTasksTableRow
                key={taskKey(task)}
                canWrite={canWrite}
                onSelectedChange={(checked) => toggleTask(task, checked)}
                returnSearch={returnSearch}
                selected={selectedSet.has(taskKey(task))}
                selectedIds={selectedIds}
                task={task}
                workspaceSlug={workspaceSlug}
              />
            ))}
          </TableBody>
        </Table>
      <div className="space-y-2 md:hidden">
        {tasks.map((task) => (
          <MyTasksTaskCard
            key={taskKey(task)}
            canWrite={canWrite}
            onSelectedChange={(checked) => toggleTask(task, checked)}
            returnSearch={returnSearch}
            selected={selectedSet.has(taskKey(task))}
            selectedIds={selectedIds}
            task={task}
            workspaceSlug={workspaceSlug}
          />
        ))}
      </div>
      </section>
    </EditFeedbackProvider>
  )
}

function SortHead({
  active,
  label,
  onSortChange,
  sort: sortKey,
}: {
  active: boolean
  label: string
  onSortChange?: (sort: string) => void
  sort: string
}) {
  if (!onSortChange) {
    return <TableHead>{label}</TableHead>
  }
  return (
    <TableHead>
      <button
        aria-label={`按${label}排序`}
        aria-pressed={active}
        className="font-medium underline-offset-4 hover:underline"
        onClick={() => onSortChange(sortKey)}
        type="button"
      >
        {label}
      </button>
    </TableHead>
  )
}

function MyTasksTableRow({
  canWrite,
  onSelectedChange,
  returnSearch,
  selected,
  selectedIds,
  task,
  workspaceSlug,
}: {
  canWrite: boolean
  onSelectedChange: (checked: boolean) => void
  returnSearch: string
  selected: boolean
  selectedIds: string[]
  task: ProjectWorkbenchTask
  workspaceSlug: string
}) {
  const { i18n, t } = useTranslation()
  const isDeleted = task.status === "deleted"
  const saveReturnState = () =>
    saveMyTasksReturnState(returnSearch, {
      focusId: taskKey(task),
      scrollTop: window.scrollY,
      selectedIds,
    })
  return (
    <TableRow
      className={isDeleted ? "opacity-50" : undefined}
      data-state={selected ? "selected" : undefined}
    >
      {canWrite ? (
        <TableCell>
          <Checkbox
            aria-label={t("myTasks.bulk.selectTask", {
              task: taskDisplayRef(task, i18n.language),
            })}
            checked={selected}
            disabled={isDeleted}
            onCheckedChange={(checked) => onSelectedChange(checked === true)}
          />
        </TableCell>
      ) : null}
      <TableCell>
        <TaskDetailLink
          returnSearch={returnSearch}
          onNavigate={saveReturnState}
          task={task}
          workspaceSlug={workspaceSlug}
        >
          {taskDisplayRef(task, i18n.language)}
        </TaskDetailLink>
      </TableCell>
      <TableCell className="max-w-lg min-w-48 truncate">
        {task.title}
        {isDeleted ? (
          <Badge className="ml-2" variant="outline">
            {t("myTasks.deleted")}
          </Badge>
        ) : null}
      </TableCell>
      <TableCell>
		<Badge variant="outline">
		  {task.recurrence_info?.materialization === "projected"
			? t("taskSeries.occurrence.projected")
			: taskStatusLabel(task.status, t)}
		</Badge>
      </TableCell>
      <TableCell>
        {task.priority ? (priorityLabel[task.priority] ?? task.priority) : "-"}
      </TableCell>
      <TableCell className="text-xs whitespace-nowrap text-muted-foreground">
        {formatDue(task.due)}
      </TableCell>
      <TableCell className="max-w-32 truncate">
        {task.project ? (
          <Link
            className="text-foreground underline-offset-4 hover:underline"
            params={{
              workspaceSlug,
              projectSlug: task.project,
            }}
            to="/workspaces/$workspaceSlug/projects/$projectSlug"
          >
            {task.project}
          </Link>
        ) : (
          "-"
        )}
      </TableCell>
      <TableCell>
        {task.project ? (
          <TaskRowActions
            canWrite={canWrite}
            displayRef={taskDisplayRef(task, i18n.language)}
            myTasksReturnSearch={returnSearch}
            onNavigateFromList={saveReturnState}
            projectSlug={task.project}
            recurrenceInfo={task.recurrence_info}
            start={task.start}
            status={task.status}
            taskRef={taskRouteRef(task)}
            workspaceSlug={workspaceSlug}
          />
        ) : null}
      </TableCell>
    </TableRow>
  )
}

function MyTasksTaskCard({
  canWrite,
  onSelectedChange,
  returnSearch,
  selected,
  selectedIds,
  task,
  workspaceSlug,
}: {
  canWrite: boolean
  onSelectedChange: (checked: boolean) => void
  returnSearch: string
  selected: boolean
  selectedIds: string[]
  task: ProjectWorkbenchTask
  workspaceSlug: string
}) {
  const { i18n, t } = useTranslation()
  const saveReturnState = () =>
    saveMyTasksReturnState(returnSearch, {
      focusId: taskKey(task),
      scrollTop: window.scrollY,
      selectedIds,
    })
  return (
    <article
      className="rounded-lg border bg-card p-3"
      data-state={selected ? "selected" : undefined}
    >
      <div className="flex items-center justify-between gap-2 text-xs">
		<div className="flex min-w-0 items-center gap-2">
		  {canWrite ? (
		    <Checkbox
		      aria-label={t("myTasks.bulk.selectTask", {
		        task: taskDisplayRef(task, i18n.language),
		      })}
		      checked={selected}
		      disabled={task.status === "deleted"}
		      onCheckedChange={(checked) => onSelectedChange(checked === true)}
		    />
		  ) : null}
		  <TaskDetailLink
		    returnSearch={returnSearch}
		    onNavigate={saveReturnState}
		    task={task}
		    workspaceSlug={workspaceSlug}
		  >
		    {taskDisplayRef(task, i18n.language)}
		  </TaskDetailLink>
		</div>
		<Badge variant="outline">
		  {task.recurrence_info?.materialization === "projected"
			? t("taskSeries.occurrence.projected")
			: taskStatusLabel(task.status, t)}
		</Badge>
      </div>
      <div className="mt-2 text-sm font-medium">{task.title}</div>
      <div className="mt-1 truncate text-xs text-muted-foreground">
        {task.project ? (
          <Link
            className="text-foreground underline-offset-4 hover:underline"
            params={{
              workspaceSlug,
              projectSlug: task.project,
            }}
            to="/workspaces/$workspaceSlug/projects/$projectSlug"
          >
            {task.project}
          </Link>
        ) : (
          "-"
        )}
      </div>
      {task.project ? (
        <div className="mt-3 flex justify-end border-t pt-2">
          <TaskRowActions
            canWrite={canWrite}
            displayRef={taskDisplayRef(task, i18n.language)}
            myTasksReturnSearch={returnSearch}
            onNavigateFromList={saveReturnState}
            projectSlug={task.project}
            recurrenceInfo={task.recurrence_info}
            start={task.start}
            status={task.status}
            taskRef={taskRouteRef(task)}
            workspaceSlug={workspaceSlug}
          />
        </div>
      ) : null}
    </article>
  )
}

function TaskDetailLink({
  children,
  onNavigate,
  returnSearch,
  task,
  workspaceSlug,
}: {
  children: React.ReactNode
  onNavigate?: () => void
  returnSearch: string
  task: ProjectWorkbenchTask
  workspaceSlug: string
}) {
  if (!task.project) {
    return <span className="text-foreground">{children}</span>
  }

  return (
    <Link
      className="text-foreground underline-offset-4 hover:underline"
      data-my-task-focus={taskKey(task)}
      onClick={onNavigate}
      params={{
        projectSlug: task.project,
        taskRef: taskRouteRef(task),
        workspaceSlug,
      }}
      search={{ from: "my-tasks", my_tasks_search: returnSearch }}
      to="/workspaces/$workspaceSlug/projects/$projectSlug/tasks/$taskRef"
    >
      {children}
    </Link>
  )
}

function taskKey(task: ProjectWorkbenchTask): string {
  return task.id || task.uuid || ""
}

function formatDue(due: ProjectWorkbenchTask["due"]): string {
  if (due === null || due === undefined) return "-"
  const ts =
    typeof due === "number"
      ? due
      : (() => {
          const parsed = Date.parse(due)
          return Number.isNaN(parsed) ? null : Math.floor(parsed / 1000)
        })()
  if (ts === null) return "-"
  const date = new Date(ts * 1000)
  const y = date.getFullYear()
  const m = String(date.getMonth() + 1).padStart(2, "0")
  const d = String(date.getDate()).padStart(2, "0")
  return `${y}-${m}-${d}`
}

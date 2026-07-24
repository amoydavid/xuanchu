import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import type {
  ProjectWorkbenchAssignee,
  ProjectWorkbenchTask,
} from "../api/project-api"

type AssigneeWorkloadSummaryProps = {
  now: number
  onFilterAssignee: (assigneeRef: string) => void
  onFilterUnassigned: () => void
  tasks: ProjectWorkbenchTask[]
}

type WorkloadRow = {
  key: string
  label: string
  filterRef?: string
  total: number
  open: number
  overdue: number
  highPriority: number
  dueSoon: number
  completed: number
  unassigned?: boolean
}

export function AssigneeWorkloadSummary({
  now,
  onFilterAssignee,
  onFilterUnassigned,
  tasks,
}: AssigneeWorkloadSummaryProps) {
  const { t } = useTranslation()
  const rows = buildWorkloadRows(tasks, now)
  if (rows.length === 0) {
    return null
  }

  return (
    <section className="space-y-3">
      <div className="flex items-center justify-between gap-3">
        <h2 className="text-sm font-medium">
          {t("projectReadonly.assigneeSummary")}
        </h2>
        <span className="text-xs text-muted-foreground">
          {t("projectWorkbench.workload.assigneeCount", {
            count: rows.length,
          })}
        </span>
      </div>
      <div className="divide-y">
        {rows.slice(0, 8).map((row) => {
          const ratio =
            row.total > 0 ? Math.round((row.completed / row.total) * 100) : 0
          return (
            <Button
              aria-label={t("projectWorkbench.workload.filterAssignee", {
                name: row.label,
              })}
              className="grid h-auto w-full grid-cols-[minmax(8rem,1fr)_minmax(12rem,2fr)] items-center gap-4 rounded-none px-0 py-3 text-left hover:bg-transparent"
              key={row.key}
              onClick={() =>
                row.unassigned
                  ? onFilterUnassigned()
                  : onFilterAssignee(row.filterRef ?? row.key)
              }
              type="button"
              variant="ghost"
            >
              <span className="min-w-0">
                <span className="block truncate text-sm font-medium">
                  {row.label}
                </span>
                <span className="mt-1 block text-xs text-muted-foreground">
                  {t("projectWorkbench.workload.totalTasks", {
                    count: row.total,
                  })}
                </span>
              </span>
              <span className="min-w-0 space-y-2">
                <span className="flex flex-wrap gap-x-4 gap-y-1 text-xs text-muted-foreground">
                  <span>
                    {t("projectWorkbench.workload.openTasks", {
                      count: row.open,
                    })}
                  </span>
                  <span>
                    {t("projectWorkbench.workload.overdueTasks", {
                      count: row.overdue,
                    })}
                  </span>
                  <span>
                    {t("projectWorkbench.workload.highPriorityTasks", {
                      count: row.highPriority,
                    })}
                  </span>
                  <span>
                    {t("projectWorkbench.workload.dueSoonTasks", {
                      count: row.dueSoon,
                    })}
                  </span>
                </span>
                <span className="block h-1.5 overflow-hidden bg-muted">
                  <span
                    className="block h-full bg-primary"
                    style={{ width: `${ratio}%` }}
                  />
                </span>
              </span>
            </Button>
          )
        })}
      </div>
    </section>
  )
}

function buildWorkloadRows(
  tasks: ProjectWorkbenchTask[],
  now: number
): WorkloadRow[] {
  const rows = new Map<string, WorkloadRow>()
  const dueSoonEnd = now + 7 * 24 * 60 * 60
  for (const item of tasks) {
    const assignees = item.assignees?.length
      ? item.assignees
      : [{ id: "__unassigned__", name: "未分配" }]
    for (const assignee of assignees) {
      const key = assigneeRef(assignee)
      const row =
        rows.get(key) ??
        ({
          key,
          label: assigneeLabel(assignee),
          filterRef:
            assignee.id || assignee.email || assignee.name,
          total: 0,
          open: 0,
          overdue: 0,
          highPriority: 0,
          dueSoon: 0,
          completed: 0,
          unassigned: key === "__unassigned__",
        } satisfies WorkloadRow)
      row.total += 1
      if (isCompleted(item)) {
        row.completed += 1
      } else {
        row.open += 1
        const due = unixLikeToNumber(item.due)
        if (due !== null && due < now) {
          row.overdue += 1
        }
        if (due !== null && due >= now && due <= dueSoonEnd) {
          row.dueSoon += 1
        }
        if (item.priority === "H") {
          row.highPriority += 1
        }
      }
      rows.set(key, row)
    }
  }
  return Array.from(rows.values()).sort((a, b) => {
    if (a.unassigned !== b.unassigned) {
      return a.unassigned ? -1 : 1
    }
    return (
      b.open - a.open || b.overdue - a.overdue || a.label.localeCompare(b.label)
    )
  })
}

function assigneeRef(assignee: ProjectWorkbenchAssignee): string {
  return (
    assignee.id ||
    assignee.email ||
    assignee.name ||
    "__unassigned__"
  )
}

function assigneeLabel(assignee: ProjectWorkbenchAssignee): string {
  return assignee.display_name || assignee.name || assignee.email || "未分配"
}

function isCompleted(task: ProjectWorkbenchTask): boolean {
  return task.status === "completed" || task.status === "deleted"
}

function unixLikeToNumber(value: string | number | null | undefined) {
  if (typeof value === "number") {
    return value
  }
  if (typeof value === "string") {
    const parsed = Date.parse(value)
    if (!Number.isNaN(parsed)) {
      return Math.floor(parsed / 1000)
    }
  }
  return null
}

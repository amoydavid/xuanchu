import type { ProjectReadonlyTask } from "./project-readonly-api"

export type ProjectTask = ProjectReadonlyTask

export type ProjectStats = {
  active: number
  completed: number
  highPriority: number
  overdue: number
  pending: number
  total: number
}

export type AssigneeSummary = {
  key: string
  label: string
  open: number
  overdue: number
  total: number
}

const closedStatuses = new Set(["completed", "deleted"])

export function buildProjectStats(
  tasks: ProjectTask[],
  now: number
): ProjectStats {
  return tasks.reduce<ProjectStats>(
    (stats, task) => {
      if (task.status === "pending") {
        stats.pending += 1
      }
      if (task.status === "active") {
        stats.active += 1
      }
      if (task.status === "completed") {
        stats.completed += 1
      }
      if (task.priority === "H") {
        stats.highPriority += 1
      }
      if (isOverdue(task, now)) {
        stats.overdue += 1
      }
      stats.total += 1
      return stats
    },
    {
      active: 0,
      completed: 0,
      highPriority: 0,
      overdue: 0,
      pending: 0,
      total: 0,
    }
  )
}

export function summarizeAssignees(
  tasks: ProjectTask[],
  now: number,
  unassignedLabel: string
): AssigneeSummary[] {
  const summaries = new Map<string, AssigneeSummary>()

  for (const task of tasks) {
    const assignees =
      task.assignees && task.assignees.length > 0
        ? task.assignees
        : [{ user_id: "unassigned", name: unassignedLabel }]
    for (const assignee of assignees) {
      const key = assignee.user_id || assignee.id || assignee.name || "unknown"
      const label = assignee.name || assignee.email || key
      const current =
        summaries.get(key) ??
        ({
          key,
          label,
          open: 0,
          overdue: 0,
          total: 0,
        } satisfies AssigneeSummary)
      current.total += 1
      if (!isClosed(task)) {
        current.open += 1
      }
      if (isOverdue(task, now)) {
        current.overdue += 1
      }
      summaries.set(key, current)
    }
  }

  return [...summaries.values()].sort(
    (left, right) =>
      right.open - left.open ||
      right.overdue - left.overdue ||
      right.total - left.total ||
      left.label.localeCompare(right.label)
  )
}

export function isClosed(task: ProjectTask): boolean {
  return closedStatuses.has(task.status)
}

export function isOverdue(task: ProjectTask, now: number): boolean {
  return typeof task.due === "number" && task.due < now && !isClosed(task)
}

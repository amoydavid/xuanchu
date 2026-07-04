import { useQuery } from "@tanstack/react-query"

import {
  getTask,
  getTaskAudit,
  type ProjectTask,
  type TaskAuditEntry,
} from "../api/task-api"

export const taskQueryKeys = {
  task: (workspaceSlug: string, taskRef: string) =>
    ["task", workspaceSlug, taskRef] as const,
  audit: (workspaceSlug: string, taskRef: string) =>
    ["task", workspaceSlug, taskRef, "audit"] as const,
}

export function useTaskDetailQuery(
  workspaceSlug: string,
  taskRef: string,
  initialData?: ProjectTask
) {
  return useQuery({
    queryKey: taskQueryKeys.task(workspaceSlug, taskRef),
    queryFn: () => getTask(workspaceSlug, taskRef),
    enabled: workspaceSlug.length > 0 && taskRef.length > 0,
    initialData,
  })
}

export function useTaskAuditQuery(
  workspaceSlug: string,
  taskRef: string
) {
  return useQuery<TaskAuditEntry[]>({
    queryKey: taskQueryKeys.audit(workspaceSlug, taskRef),
    queryFn: () => getTaskAudit(workspaceSlug, taskRef),
    enabled: workspaceSlug.length > 0 && taskRef.length > 0,
  })
}

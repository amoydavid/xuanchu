import { useQuery, useQueryClient } from "@tanstack/react-query"

import {
  getTask,
  getTaskAudit,
  listTaskChildren,
  type ProjectTask,
  type TaskAuditEntry,
} from "../api/task-api"
import {
  canonicalTaskRouteRef,
  taskStableCacheRef,
} from "../tasks/task-reference"

export const taskQueryKeys = {
  task: (workspaceSlug: string, taskRef: string) =>
    ["task", workspaceSlug, taskRef] as const,
  audit: (workspaceSlug: string, taskRef: string) =>
    ["task", workspaceSlug, taskRef, "audit"] as const,
  children: (workspaceSlug: string, taskRef: string, includeClosed: boolean) =>
    [
      "task",
      workspaceSlug,
      taskRef,
      "children",
      includeClosed ? "all" : "open",
    ] as const,
  childrenPrefix: (workspaceSlug: string, taskRef: string) =>
    ["task", workspaceSlug, taskRef, "children"] as const,
}

export function useTaskDetailQuery(
  workspaceSlug: string,
  taskRef: string,
  initialData?: ProjectTask
) {
  const queryClient = useQueryClient()
  return useQuery({
    queryKey: taskQueryKeys.task(workspaceSlug, taskRef),
    queryFn: async () => {
      const loaded = await getTask(workspaceSlug, taskRef)
      const aliases = new Set([
        taskRef,
        taskStableCacheRef(loaded),
        canonicalTaskRouteRef(loaded) ?? "",
      ])
      for (const alias of aliases) {
        if (alias) {
          queryClient.setQueryData(
            taskQueryKeys.task(workspaceSlug, alias),
            loaded
          )
        }
      }
      return loaded
    },
    enabled: workspaceSlug.length > 0 && taskRef.length > 0,
    initialData,
  })
}

export function useTaskAuditQuery(workspaceSlug: string, taskRef: string) {
  return useQuery<TaskAuditEntry[]>({
    queryKey: taskQueryKeys.audit(workspaceSlug, taskRef),
    queryFn: () => getTaskAudit(workspaceSlug, taskRef),
    enabled: workspaceSlug.length > 0 && taskRef.length > 0,
  })
}

export function useTaskChildrenQuery(
  workspaceSlug: string,
  taskRef: string,
  includeClosed: boolean
) {
  return useQuery<ProjectTask[]>({
    queryKey: taskQueryKeys.children(workspaceSlug, taskRef, includeClosed),
    queryFn: () => listTaskChildren(workspaceSlug, taskRef, includeClosed),
    enabled: workspaceSlug.length > 0 && taskRef.length > 0,
  })
}

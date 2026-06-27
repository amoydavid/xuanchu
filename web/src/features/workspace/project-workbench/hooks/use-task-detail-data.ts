import { useQuery } from "@tanstack/react-query"

import { getTask, type ProjectTask } from "../api/task-api"

export const taskQueryKeys = {
  task: (workspaceSlug: string, taskRef: string) =>
    ["task", workspaceSlug, taskRef] as const,
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

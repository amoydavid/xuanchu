import {
  useInfiniteQuery,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query"

import {
  getTask,
  getTaskActivity,
  listTaskChildren,
  type ProjectTask,
} from "../api/task-api"
import {
  canonicalTaskRouteRef,
  taskStableCacheRef,
} from "../tasks/task-reference"

export const taskQueryKeys = {
  task: (workspaceSlug: string, taskRef: string) =>
    ["task", workspaceSlug, taskRef] as const,
  activity: (workspaceSlug: string, taskRef: string) =>
    ["task", workspaceSlug, taskRef, "activity"] as const,
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

export function useTaskActivityQuery(workspaceSlug: string, taskRef: string) {
  return useInfiniteQuery({
    queryKey: taskQueryKeys.activity(workspaceSlug, taskRef),
    queryFn: ({ pageParam }) =>
      getTaskActivity(workspaceSlug, taskRef, {
        limit: 30,
        cursor: pageParam || undefined,
      }),
    initialPageParam: "",
    getNextPageParam: (lastPage) => lastPage.next_cursor ?? undefined,
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

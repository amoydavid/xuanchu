import { useMutation, useQueryClient } from "@tanstack/react-query"

import type { ProjectTaskFilterParams } from "../api/project-api"
import {
  addTaskAnnotation,
  addTaskLink,
  createTask,
  deleteTask,
  deleteTaskAnnotation,
  deleteTaskLink,
  doneTask,
  modifyTask,
  startTask,
  stopTask,
  type TaskAnnotationInput,
  type TaskCreateInput,
  type TaskLinkInput,
  type TaskModifyInput,
} from "../api/task-api"
import { projectQueryKeys } from "./use-project-data"
import { taskQueryKeys } from "./use-task-detail-data"

export type TaskAction = "start" | "stop" | "done" | "delete"

function filterKey(filters?: ProjectTaskFilterParams | string): string | undefined {
  if (!filters) {
    return undefined
  }
  if (typeof filters === "string") {
    return filters || undefined
  }
  if (filters instanceof URLSearchParams) {
    const raw = filters.toString()
    return raw || undefined
  }
  const query = new URLSearchParams()
  for (const [key, value] of Object.entries(filters)) {
    if (value !== undefined && value !== null && value !== "") {
      query.set(key, String(value))
    }
  }
  const raw = query.toString()
  return raw || undefined
}

function projectTasksQueryKey(
  workspaceSlug: string,
  projectSlug: string,
  filters?: ProjectTaskFilterParams | string
) {
  const key = filterKey(filters)
  const prefix = projectQueryKeys.projectTasksPrefix(workspaceSlug, projectSlug)
  return key ? [...prefix, key] as const : prefix
}

function invalidateProjectTaskSurface(
  queryClient: ReturnType<typeof useQueryClient>,
  workspaceSlug: string,
  projectSlug: string
) {
  void queryClient.invalidateQueries({
    queryKey: projectQueryKeys.projectTasksPrefix(workspaceSlug, projectSlug),
  })
  void queryClient.invalidateQueries({
    queryKey: projectQueryKeys.project(workspaceSlug, projectSlug),
  })
}

export function useCreateTaskMutation(
  workspaceSlug: string,
  projectSlug: string,
  filters?: ProjectTaskFilterParams | string
) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (input: TaskCreateInput) => createTask(workspaceSlug, input),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: projectTasksQueryKey(workspaceSlug, projectSlug, filters),
      })
      void queryClient.invalidateQueries({
        queryKey: projectQueryKeys.projectTasksPrefix(workspaceSlug, projectSlug),
      })
      void queryClient.invalidateQueries({
        queryKey: projectQueryKeys.project(workspaceSlug, projectSlug),
      })
    },
  })
}

export function useModifyTaskMutation(
  workspaceSlug: string,
  projectSlug: string,
  taskRef?: string
) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (input: TaskModifyInput & { taskRef?: string }) => {
      const ref = input.taskRef ?? taskRef
      if (!ref) {
        throw new Error("taskRef is required")
      }
      const { taskRef: _taskRef, ...payload } = input
      return modifyTask(workspaceSlug, ref, payload)
    },
    onSuccess: (_task, input) => {
      const ref = input.taskRef ?? taskRef
      if (ref) {
        void queryClient.invalidateQueries({
          queryKey: taskQueryKeys.task(workspaceSlug, ref),
        })
      }
      void queryClient.invalidateQueries({
        queryKey: projectQueryKeys.projectTasksPrefix(workspaceSlug, projectSlug),
      })
    },
  })
}

export function useTaskActionMutation(
  workspaceSlug: string,
  projectSlug: string,
  action: TaskAction
) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (taskRef: string) => {
      if (action === "start") {
        return startTask(workspaceSlug, taskRef)
      }
      if (action === "stop") {
        return stopTask(workspaceSlug, taskRef)
      }
      if (action === "done") {
        return doneTask(workspaceSlug, taskRef)
      }
      return deleteTask(workspaceSlug, taskRef)
    },
    onSuccess: (_task, taskRef) => {
      void queryClient.invalidateQueries({
        queryKey: taskQueryKeys.task(workspaceSlug, taskRef),
      })
      invalidateProjectTaskSurface(queryClient, workspaceSlug, projectSlug)
      void queryClient.invalidateQueries({
        queryKey: projectQueryKeys.projectTimeline(workspaceSlug, projectSlug),
      })
    },
  })
}

export function useTaskAnnotationMutations(
  workspaceSlug: string,
  projectSlug: string,
  taskRef: string
) {
  const queryClient = useQueryClient()
  const invalidate = () => {
    void queryClient.invalidateQueries({
      queryKey: taskQueryKeys.task(workspaceSlug, taskRef),
    })
    void queryClient.invalidateQueries({
      queryKey: projectQueryKeys.projectTimeline(workspaceSlug, projectSlug),
    })
  }

  return {
    add: useMutation({
      mutationFn: (input: TaskAnnotationInput) =>
        addTaskAnnotation(workspaceSlug, taskRef, input),
      onSuccess: invalidate,
    }),
    remove: useMutation({
      mutationFn: (annotationID: string) =>
        deleteTaskAnnotation(workspaceSlug, taskRef, annotationID),
      onSuccess: invalidate,
    }),
  }
}

export function useTaskLinkMutations(
  workspaceSlug: string,
  projectSlug: string,
  taskRef: string
) {
  const queryClient = useQueryClient()
  const invalidate = () => {
    void queryClient.invalidateQueries({
      queryKey: taskQueryKeys.task(workspaceSlug, taskRef),
    })
    void queryClient.invalidateQueries({
      queryKey: projectQueryKeys.projectTasksPrefix(workspaceSlug, projectSlug),
    })
  }

  return {
    add: useMutation({
      mutationFn: (input: TaskLinkInput) =>
        addTaskLink(workspaceSlug, taskRef, input),
      onSuccess: invalidate,
    }),
    remove: useMutation({
      mutationFn: (linkID: string) =>
        deleteTaskLink(workspaceSlug, taskRef, linkID),
      onSuccess: invalidate,
    }),
  }
}

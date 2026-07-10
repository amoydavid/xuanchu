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
  importTasks,
  modifyTask,
  reopenTask,
  startTask,
  stopTask,
  updateTaskAnnotation,
  updateTaskLink,
  type TaskAnnotationInput,
  type TaskCreateInput,
  type TaskImportTask,
  type TaskLinkInput,
  type TaskModifyInput,
} from "../api/task-api"
import { projectQueryKeys } from "./use-project-data"
import { taskQueryKeys } from "./use-task-detail-data"
import { useEditFeedback } from "../shared/edit-feedback"

export type TaskAction = "start" | "stop" | "done" | "reopen" | "delete"

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
  const feedback = useEditFeedback()
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
      feedback.success("已创建：任务")
    },
  })
}

// 子任务创建：复用 createTask，但 parent 指向父任务 UUID。
// 成功后刷新父任务详情、子任务列表（open/all 两个 key）与项目任务列表前缀。
export function useCreateSubTaskMutation(
  workspaceSlug: string,
  projectSlug: string,
  parentRef: string
) {
  const queryClient = useQueryClient()
  const feedback = useEditFeedback()
  return useMutation({
    mutationFn: (input: TaskCreateInput) => createTask(workspaceSlug, input),
    onSuccess: () => {
      // 父任务详情（children 计数等可能体现在详情或摘要里）。
      void queryClient.invalidateQueries({
        queryKey: taskQueryKeys.task(workspaceSlug, parentRef),
      })
      // 子任务列表 open/all 两个 key 都失效。
      void queryClient.invalidateQueries({
        queryKey: taskQueryKeys.childrenPrefix(workspaceSlug, parentRef),
      })
      // 项目任务列表（子任务也会出现在项目列表里）。
      void queryClient.invalidateQueries({
        queryKey: projectQueryKeys.projectTasksPrefix(workspaceSlug, projectSlug),
      })
      feedback.success("已创建：子任务")
    },
  })
}

export function useModifyTaskMutation(
  workspaceSlug: string,
  projectSlug: string,
  taskRef?: string
) {
  const queryClient = useQueryClient()
  const feedback = useEditFeedback()
  return useMutation({
    mutationFn: (input: TaskModifyInput & { taskRef?: string }) => {
      const ref = input.taskRef ?? taskRef
      if (!ref) {
        throw new Error("taskRef is required")
      }
      const payload = { ...input }
      delete payload.taskRef
      return modifyTask(workspaceSlug, ref, payload)
    },
    onSuccess: (_task, input) => {
      const ref = input.taskRef ?? taskRef
      if (ref) {
        void queryClient.invalidateQueries({
          queryKey: taskQueryKeys.task(workspaceSlug, ref),
        })
        // 任务字段变更会产生新的 audit 历史，刷新详情页变更历史。
        void queryClient.invalidateQueries({
          queryKey: taskQueryKeys.audit(workspaceSlug, ref),
        })
      }
      void queryClient.invalidateQueries({
        queryKey: projectQueryKeys.projectTasksPrefix(workspaceSlug, projectSlug),
      })
      feedback.success("已保存：任务")
    },
  })
}

export function useTaskActionMutation(
  workspaceSlug: string,
  projectSlug: string,
  action: TaskAction
) {
  const queryClient = useQueryClient()
  const feedback = useEditFeedback()
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
      if (action === "reopen") {
        return reopenTask(workspaceSlug, taskRef)
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
      feedback.success(taskActionSuccessLabel(action))
    },
  })
}

export function useImportTasksMutation(
  workspaceSlug: string,
  projectSlug: string
) {
  const queryClient = useQueryClient()
  const feedback = useEditFeedback()
  return useMutation({
    mutationFn: (tasks: TaskImportTask[]) =>
      importTasks(workspaceSlug, projectSlug, tasks),
    onSuccess: (result) => {
      invalidateProjectTaskSurface(queryClient, workspaceSlug, projectSlug)
      void queryClient.invalidateQueries({
        queryKey: projectQueryKeys.projectTimeline(workspaceSlug, projectSlug),
      })
      feedback.success(`已导入：${result.imported} 个任务`)
    },
  })
}

export function useTaskAnnotationMutations(
  workspaceSlug: string,
  projectSlug: string,
  taskRef: string
) {
  const queryClient = useQueryClient()
  const feedback = useEditFeedback()
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
      onSuccess: () => {
        invalidate()
        feedback.success("已添加：注解")
      },
    }),
    remove: useMutation({
      mutationFn: (annotationID: string) =>
        deleteTaskAnnotation(workspaceSlug, taskRef, annotationID),
      onSuccess: () => {
        invalidate()
        feedback.success("已删除：注解")
      },
    }),
    update: useMutation({
      mutationFn: ({
        annotationID,
        input,
      }: {
        annotationID: string
        input: TaskAnnotationInput
      }) => updateTaskAnnotation(workspaceSlug, taskRef, annotationID, input),
      onSuccess: () => {
        invalidate()
        feedback.success("已保存：注解")
      },
    }),
  }
}

export function useTaskLinkMutations(
  workspaceSlug: string,
  projectSlug: string,
  taskRef: string
) {
  const queryClient = useQueryClient()
  const feedback = useEditFeedback()
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
      onSuccess: () => {
        invalidate()
        feedback.success("已添加：链接")
      },
    }),
    remove: useMutation({
      mutationFn: (linkID: string) =>
        deleteTaskLink(workspaceSlug, taskRef, linkID),
      onSuccess: () => {
        invalidate()
        feedback.success("已删除：链接")
      },
    }),
    update: useMutation({
      mutationFn: ({ input, linkID }: { input: TaskLinkInput; linkID: string }) =>
        updateTaskLink(workspaceSlug, taskRef, linkID, input),
      onSuccess: () => {
        invalidate()
        feedback.success("已保存：链接")
      },
    }),
  }
}

function taskActionSuccessLabel(action: TaskAction): string {
  if (action === "start") {
    return "已开始：任务"
  }
  if (action === "stop") {
    return "已停止：任务"
  }
  if (action === "done") {
    return "已完成：任务"
  }
  if (action === "reopen") {
    return "已重新打开：任务"
  }
  return "已删除：任务"
}

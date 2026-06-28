import { useMutation, useQueryClient } from "@tanstack/react-query"

import {
  createProject,
  modifyProject,
  transitionProject,
  type ProjectCreateInput,
  type ProjectModifyInput,
  type ProjectStatus,
} from "../api/project-api"
import { projectQueryKeys } from "./use-project-data"
import { useEditFeedback } from "../shared/edit-feedback"

export function useCreateProjectMutation(workspaceSlug: string) {
  const queryClient = useQueryClient()
  const feedback = useEditFeedback()
  return useMutation({
    mutationFn: (input: ProjectCreateInput) =>
      createProject(workspaceSlug, input),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: projectQueryKeys.projectsPrefix(workspaceSlug),
      })
      feedback.success("已创建：项目")
    },
  })
}

export function useModifyProjectMutation(
  workspaceSlug: string,
  projectSlug: string
) {
  const queryClient = useQueryClient()
  const feedback = useEditFeedback()
  return useMutation({
    mutationFn: (input: ProjectModifyInput) =>
      modifyProject(workspaceSlug, projectSlug, input),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: projectQueryKeys.project(workspaceSlug, projectSlug),
      })
      void queryClient.invalidateQueries({
        queryKey: projectQueryKeys.projectsPrefix(workspaceSlug),
      })
      feedback.success("已保存：项目")
    },
  })
}

export function useTransitionProjectMutation(
  workspaceSlug: string,
  projectSlug: string
) {
  const queryClient = useQueryClient()
  const feedback = useEditFeedback()
  return useMutation({
    mutationFn: (status: ProjectStatus | string) =>
      transitionProject(workspaceSlug, projectSlug, status),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: projectQueryKeys.project(workspaceSlug, projectSlug),
      })
      void queryClient.invalidateQueries({
        queryKey: projectQueryKeys.projectsPrefix(workspaceSlug),
      })
      void queryClient.invalidateQueries({
        queryKey: projectQueryKeys.projectTimeline(workspaceSlug, projectSlug),
      })
      void queryClient.invalidateQueries({
        queryKey: projectQueryKeys.projectTasksPrefix(
          workspaceSlug,
          projectSlug
        ),
      })
      feedback.success("已更新：项目状态")
    },
  })
}

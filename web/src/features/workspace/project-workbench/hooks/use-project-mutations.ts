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

export function useCreateProjectMutation(workspaceSlug: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (input: ProjectCreateInput) =>
      createProject(workspaceSlug, input),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: projectQueryKeys.projectsPrefix(workspaceSlug),
      })
    },
  })
}

export function useModifyProjectMutation(
  workspaceSlug: string,
  projectSlug: string
) {
  const queryClient = useQueryClient()
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
    },
  })
}

export function useTransitionProjectMutation(
  workspaceSlug: string,
  projectSlug: string
) {
  const queryClient = useQueryClient()
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
    },
  })
}

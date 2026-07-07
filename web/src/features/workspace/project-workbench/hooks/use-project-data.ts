import { useQuery } from "@tanstack/react-query"

import {
  getProject,
  getProjectTaskSummary,
  getProjectTasks,
  getProjects,
  getProjectTimeline,
  type ProjectTaskFilterParams,
  type ProjectStatus,
} from "../api/project-api"

export const projectQueryKeys = {
  projects: (
    workspaceSlug: string,
    statusFilter?: ProjectStatus | "all" | string
  ) => ["projects", workspaceSlug, statusFilter] as const,
  projectsPrefix: (workspaceSlug: string) =>
    ["projects", workspaceSlug] as const,
  project: (workspaceSlug: string, projectSlug: string) =>
    ["project", workspaceSlug, projectSlug] as const,
  projectTasksPrefix: (workspaceSlug: string, projectSlug: string) =>
    ["project", workspaceSlug, projectSlug, "tasks"] as const,
  projectTasks: (
    workspaceSlug: string,
    projectSlug: string,
    filters?: ProjectTaskFilterParams | string
  ) =>
    [
      "project",
      workspaceSlug,
      projectSlug,
      "tasks",
      projectTaskFilterKey(filters),
    ] as const,
  projectTimeline: (workspaceSlug: string, projectSlug: string) =>
    ["project", workspaceSlug, projectSlug, "timeline"] as const,
  projectTaskSummary: (workspaceSlug: string, projectSlug: string) =>
    ["project", workspaceSlug, projectSlug, "task-summary"] as const,
}

export function useProjectsQuery(
  workspaceSlug: string,
  statusFilter?: ProjectStatus | "all" | string
) {
  return useQuery({
    queryKey: projectQueryKeys.projects(workspaceSlug, statusFilter),
    queryFn: () => getProjects(workspaceSlug, statusFilter),
    enabled: workspaceSlug.length > 0,
  })
}

export function useProjectQuery(workspaceSlug: string, projectSlug: string) {
  return useQuery({
    queryKey: projectQueryKeys.project(workspaceSlug, projectSlug),
    queryFn: () => getProject(workspaceSlug, projectSlug),
    enabled: workspaceSlug.length > 0 && projectSlug.length > 0,
  })
}

export function useProjectTimelineQuery(
  workspaceSlug: string,
  projectSlug: string
) {
  return useQuery({
    queryKey: projectQueryKeys.projectTimeline(workspaceSlug, projectSlug),
    queryFn: () => getProjectTimeline(workspaceSlug, projectSlug),
    enabled: workspaceSlug.length > 0 && projectSlug.length > 0,
  })
}

export function useProjectTaskSummaryQuery(
  workspaceSlug: string,
  projectSlug: string,
  enabled = true
) {
  return useQuery({
    queryKey: projectQueryKeys.projectTaskSummary(workspaceSlug, projectSlug),
    queryFn: () => getProjectTaskSummary(workspaceSlug, projectSlug),
    enabled: enabled && workspaceSlug.length > 0 && projectSlug.length > 0,
  })
}

export function useProjectTasksQuery(
  workspaceSlug: string,
  projectSlug: string,
  filters?: ProjectTaskFilterParams | string
) {
  const normalizedFilters =
    typeof filters === "string" ? new URLSearchParams(filters) : filters
  return useQuery({
    queryKey: projectQueryKeys.projectTasks(workspaceSlug, projectSlug, filters),
    queryFn: () => getProjectTasks(workspaceSlug, projectSlug, normalizedFilters),
    enabled: workspaceSlug.length > 0 && projectSlug.length > 0,
  })
}

function projectTaskFilterKey(
  filters?: ProjectTaskFilterParams | string
): string {
  if (!filters) {
    return ""
  }
  if (typeof filters === "string") {
    return filters
  }
  if (filters instanceof URLSearchParams) {
    return filters.toString()
  }
  const query = new URLSearchParams()
  for (const [key, value] of Object.entries(filters)) {
    if (value !== undefined && value !== null && value !== "") {
      query.set(key, String(value))
    }
  }
  return query.toString()
}

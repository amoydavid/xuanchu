import { workspaceApiGet } from "@/features/workspace/session/workspace-api"

// ProjectSummary 对齐 GET /api/v1/projects 返回的项目对象（含聚合统计）。
export interface ProjectSummary {
  id: string
  slug: string
  name: string
  description?: string
  status: string
  task_count: number
  pending_count: number
  completed_count: number
}

export function projectsListPath(all = false): string {
  return all ? "/api/v1/projects?all=true" : "/api/v1/projects"
}

export function getProjects(all = false): Promise<ProjectSummary[]> {
  return workspaceApiGet<ProjectSummary[]>(projectsListPath(all))
}

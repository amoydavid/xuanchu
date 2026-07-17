import type {
  ProjectSeriesMetrics,
  ProjectWorkbenchProject,
  ProjectWorkbenchTask,
  UserInfo,
} from "@/features/workspace/project-workbench/api/project-api"
import { workspaceApiGet } from "@/features/workspace/session/workspace-api"

export type HomeTaskReason =
  | "started"
  | "overdue"
  | "due_today"
  | "high_priority"

export type HomeTaskItem = {
  task: ProjectWorkbenchTask
  reasons: HomeTaskReason[]
}

export type HomeMyWork = {
  open_count: number
  started_count: number
  overdue_count: number
  due_today_count: number
  high_priority_open_count: number
  items: HomeTaskItem[]
}

export type HomeActorInfo = {
  type: string
  user?: UserInfo
  token?: {
    id: string
    name: string
    prefix?: string
  }
}

export type HomeProjectUpdate = {
  id: string
  project_id: string
  entry: number
  content: string
  created_by: HomeActorInfo
  created_at: number
}

export type HomeProjectAttention = {
  project: ProjectWorkbenchProject
  overdue_count: number
  high_priority_open_count: number
  wait_ready_count: number
  unassigned_open_count: number
  series_metrics: ProjectSeriesMetrics
  latest_update: HomeProjectUpdate | null
}

export type HomeView = {
  generated_at: number
  today: string
  actor_type: "user" | "tenant_access_token" | string
  my_work: HomeMyWork | null
  project_attention: HomeProjectAttention[]
}

export function getHome(): Promise<HomeView> {
  return workspaceApiGet<HomeView>("/api/v1/home")
}

import { useParams } from "@tanstack/react-router"

import { AdminWorkspaceDetailPage } from "@/features/admin/workspaces/admin-workspace-detail-page"

export function AdminWorkspaceDetailRoute() {
  const params = useParams({ strict: false }) as {
    workspaceSlug: string
  }
  return <AdminWorkspaceDetailPage workspaceSlug={params.workspaceSlug} />
}

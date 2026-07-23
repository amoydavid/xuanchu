import { useParams } from "@tanstack/react-router"

import { WorkspaceCustomFieldsPage } from "@/features/workspace/custom-fields/workspace-custom-fields-page"
import { WorkspaceSettingsNav } from "@/features/workspace/project-templates/workspace-settings-nav"
import { canConfigManage } from "@/features/workspace/project-workbench/permissions/permissions"
import { useMe } from "@/features/workspace/session/useMe"

export function WorkspaceCustomFieldsRoute() {
  const params = useParams({ strict: false }) as { workspaceSlug?: string }
  const me = useMe()
  const workspaceSlug =
    params.workspaceSlug ?? me.data?.effective_workspace.slug ?? ""
  const canManage = canConfigManage({
    role: me.data?.effective_role,
    scopes: me.data?.token.scopes,
  })
  return (
    <>
      <WorkspaceSettingsNav
        active="customFields"
        workspaceSlug={workspaceSlug}
      />
      <WorkspaceCustomFieldsPage
        canManage={canManage}
        workspaceSlug={workspaceSlug}
      />
    </>
  )
}

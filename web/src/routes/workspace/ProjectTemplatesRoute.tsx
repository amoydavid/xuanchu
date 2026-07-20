import { ProjectTemplateLibraryPage } from "@/features/workspace/project-templates/project-template-library-page"
import { WorkspaceSettingsNav } from "@/features/workspace/project-templates/workspace-settings-nav"
import { canProjectManage } from "@/features/workspace/project-workbench/permissions/permissions"
import { useMe } from "@/features/workspace/session/useMe"

export function ProjectTemplatesRoute() {
  const me = useMe()
  const workspaceSlug = me.data?.effective_workspace.slug ?? ""
  const canManage = canProjectManage({
    role: me.data?.effective_role,
    scopes: me.data?.token.scopes,
  })

  return (
    <>
      <WorkspaceSettingsNav active="projectTemplates" />
      <ProjectTemplateLibraryPage
        canManage={canManage}
        writeScopes={me.data?.token.scopes}
        workspaceSlug={workspaceSlug}
      />
    </>
  )
}

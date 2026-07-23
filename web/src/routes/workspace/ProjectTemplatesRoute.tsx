import { useState } from "react"

import { ProjectTemplateCaptureWizard } from "@/features/workspace/project-templates/capture/project-template-capture-wizard"
import { ProjectTemplateLibraryPage } from "@/features/workspace/project-templates/project-template-library-page"
import { WorkspaceSettingsNav } from "@/features/workspace/project-templates/workspace-settings-nav"
import type { ProjectWorkbenchProject } from "@/features/workspace/project-workbench/api/project-api"
import { canProjectManage } from "@/features/workspace/project-workbench/permissions/permissions"
import { useMe } from "@/features/workspace/session/useMe"

export function ProjectTemplatesRoute() {
  const me = useMe()
  const [captureSource, setCaptureSource] = useState<ProjectWorkbenchProject>()
  const workspaceSlug = me.data?.effective_workspace.slug ?? ""
  const canManage = canProjectManage({
    role: me.data?.effective_role,
    scopes: me.data?.token.scopes,
  })

  return (
    <>
      <WorkspaceSettingsNav
        active="projectTemplates"
        workspaceSlug={workspaceSlug}
      />
      <ProjectTemplateLibraryPage
        canManage={canManage}
        onCreateTemplate={setCaptureSource}
        writeScopes={me.data?.token.scopes}
        workspaceSlug={workspaceSlug}
      />
      {captureSource ? (
        <ProjectTemplateCaptureWizard
          mode="create"
          onOpenChange={(open) => {
            if (!open) setCaptureSource(undefined)
          }}
          onSaved={() => setCaptureSource(undefined)}
          open
          sourceProject={{
            id: captureSource.id,
            name: captureSource.name,
            slug: captureSource.slug,
          }}
          workspaceSlug={workspaceSlug}
        />
      ) : null}
    </>
  )
}

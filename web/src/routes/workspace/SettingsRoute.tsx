import { ConfigDefinitionsPage } from "@/pages/config-definitions-page"
import { WorkspaceSettingsNav } from "@/features/workspace/project-templates/workspace-settings-nav"
import { useMe } from "@/features/workspace/session/useMe"

// /settings 路由：workspace 级 ConfigDefinition 管理页。
// 不再走 ResourceRoute 展示 workspace config value。
export function SettingsRoute() {
  const me = useMe()
  return (
    <>
      <WorkspaceSettingsNav
        active="configDefinitions"
        workspaceSlug={me.data?.effective_workspace.slug}
      />
      <ConfigDefinitionsPage variant="workspace" />
    </>
  )
}

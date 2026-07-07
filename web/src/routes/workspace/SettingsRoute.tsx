import { ConfigDefinitionsPage } from "@/pages/config-definitions-page"

// /settings 路由：workspace 级 ConfigDefinition 管理页。
// 不再走 ResourceRoute 展示 workspace config value。
export function SettingsRoute() {
  return <ConfigDefinitionsPage variant="workspace" />
}

import { ConfigDefinitionsPage } from "@/pages/config-definitions-page"

// /projects/$projectSlug/settings/definitions
// project 级 ConfigDefinition 配置页。定义本身归属 workspace，这里只是
// project 上下文里的 project-scope 快捷控制面；权限由页面内 canConfigManage 决定，
// 不受 project archived 状态影响（定义不是项目实体的一部分）。
export function ProjectSettingsDefinitionsRoute() {
  return <ConfigDefinitionsPage variant="project" />
}

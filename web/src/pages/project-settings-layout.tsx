import { Outlet, useLocation, useNavigate } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"

import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs"

type ProjectSettingsLayoutProps = {
  projectSlug: string
  workspaceSlug: string
}

export function ProjectSettingsLayout({
  projectSlug,
  workspaceSlug,
}: ProjectSettingsLayoutProps) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const location = useLocation()
  const activeTab = activeSettingsTab(location.pathname)

  const onValueChange = (value: string) => {
    const tab = value as SettingsTab
    void navigate({
      to: settingsTabPath(tab),
      params: { projectSlug },
    })
  }

  return (
    <div className="max-w-3xl space-y-4">
      <div>
        <nav className="text-xs text-muted-foreground">
          <a
            className="hover:text-foreground"
            href={`/workspaces/${workspaceSlug}/projects/${projectSlug}`}
          >
            {projectSlug}
          </a>
          {" / "}
          <span>{t("projectSettings.title")}</span>
        </nav>
        <h1 className="mt-2 text-xl font-semibold tracking-normal">
          {t("projectSettings.title")}
        </h1>
      </div>

      <Tabs onValueChange={onValueChange} value={activeTab}>
        <TabsList>
          <TabsTrigger value="config">
            {t("projectSettings.tabConfig")}
          </TabsTrigger>
          <TabsTrigger value="definitions">
            {t("projectSettings.tabDefinitions")}
          </TabsTrigger>
          <TabsTrigger value="notes">
            {t("projectSettings.tabNotes")}
          </TabsTrigger>
        </TabsList>
      </Tabs>

      <Outlet />
    </div>
  )
}

type SettingsTab = "config" | "definitions" | "notes"

// activeSettingsTab 根据 pathname 后缀推导当前激活的 settings tab。
// 导出供测试覆盖，避免依赖完整 router 渲染。
export function activeSettingsTab(pathname: string): SettingsTab {
  if (pathname.endsWith("/notes")) return "notes"
  if (pathname.endsWith("/definitions")) return "definitions"
  return "config"
}

function settingsTabPath(tab: SettingsTab): string {
  switch (tab) {
    case "notes":
      return "/projects/$projectSlug/settings/notes"
    case "definitions":
      return "/projects/$projectSlug/settings/definitions"
    default:
      return "/projects/$projectSlug/settings/config"
  }
}

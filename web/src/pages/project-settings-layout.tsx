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
  const activeTab = location.pathname.endsWith("/notes") ? "notes" : "config"

  const onValueChange = (value: string) => {
    if (value === "notes") {
      void navigate({
        to: "/projects/$projectSlug/settings/notes",
        params: { projectSlug },
      })
    } else {
      void navigate({
        to: "/projects/$projectSlug/settings/config",
        params: { projectSlug },
      })
    }
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
          <TabsTrigger value="notes">
            {t("projectSettings.tabNotes")}
          </TabsTrigger>
        </TabsList>
      </Tabs>

      <Outlet />
    </div>
  )
}

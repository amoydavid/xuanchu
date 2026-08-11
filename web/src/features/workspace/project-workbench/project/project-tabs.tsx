import { Link } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"

import { cn } from "@/lib/utils"

export type ProjectTabKey =
  | "overview"
  | "tasks"
  | "series"
  | "activity"
  | "automations"

type ProjectTabsProps = {
  activeTab: ProjectTabKey
  className?: string
  projectSlug: string
  workspaceSlug: string
}

// ProjectTabs 在项目子页面之间提供「概览 / 任务 / 循环任务 / 活动 / 自动化」导航。
// 当前 tab 由父级根据当前路由 pathname 推断，不读取后端。
export function ProjectTabs({
  activeTab,
  className,
  projectSlug,
  workspaceSlug,
}: ProjectTabsProps) {
  const { t } = useTranslation()
  const items: ReadonlyArray<{
    key: ProjectTabKey
    label: string
    to: string
  }> = [
    {
      key: "overview",
      label: t("projectSubpages.overview"),
      to: "/workspaces/$workspaceSlug/projects/$projectSlug",
    },
    {
      key: "tasks",
      label: t("projectSubpages.tasks"),
      to: "/workspaces/$workspaceSlug/projects/$projectSlug/tasks",
    },
    {
      key: "series",
      label: t("projectSubpages.recurring"),
      to: "/workspaces/$workspaceSlug/projects/$projectSlug/series",
    },
    {
      key: "activity",
      label: t("projectSubpages.activity"),
      to: "/workspaces/$workspaceSlug/projects/$projectSlug/activity",
    },
    {
      key: "automations",
      label: t("projectSubpages.automations"),
      to: "/workspaces/$workspaceSlug/projects/$projectSlug/automations",
    },
  ]
  return (
    <nav
      aria-label={t("projectSubpages.tabs")}
      className={cn("flex items-center gap-1 overflow-x-auto", className)}
    >
      {items.map((item) => {
        const active = item.key === activeTab
        return (
          <Link
            aria-current={active ? "page" : undefined}
            className={cn(
              "shrink-0 whitespace-nowrap border-b-2 px-3 py-2 text-sm transition-colors",
              active
                ? "border-primary font-medium text-foreground"
                : "border-transparent text-muted-foreground hover:text-foreground",
            )}
            key={item.key}
            params={{ projectSlug, workspaceSlug }}
            to={item.to}
          >
            {item.label}
          </Link>
        )
      })}
    </nav>
  )
}

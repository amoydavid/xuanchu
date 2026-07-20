import { Link } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"

import { cn } from "@/lib/utils"

export function WorkspaceSettingsNav({
  active,
}: {
  active: "configDefinitions" | "projectTemplates"
}) {
  const { t } = useTranslation()
  const items = [
    {
      key: "configDefinitions" as const,
      href: "/settings",
      label: t("workspaceSettingsNav.configDefinitions"),
    },
    {
      key: "projectTemplates" as const,
      href: "/settings/project-templates",
      label: t("workspaceSettingsNav.projectTemplates"),
    },
  ]

  return (
    <nav
      aria-label={t("workspaceSettingsNav.label")}
      className="flex border-b px-6"
    >
      {items.map((item) => {
        const current = active === item.key
        return (
          <Link
            aria-current={current ? "page" : undefined}
            className={cn(
              "border-b-2 px-3 py-2 text-xs text-muted-foreground transition-colors hover:text-foreground",
              current
                ? "border-foreground font-medium text-foreground"
                : "border-transparent"
            )}
            key={item.key}
            to={item.href}
          >
            {item.label}
          </Link>
        )
      })}
    </nav>
  )
}

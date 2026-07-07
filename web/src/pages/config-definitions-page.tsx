import { useTranslation } from "react-i18next"

import { ConfigDefinitionManager } from "@/features/workspace/config/config-definition-manager"
import type { ConfigAllowedScope } from "@/features/workspace/config/config-definition-api"
import { useMe } from "@/features/workspace/session/useMe"
import { canConfigManage } from "@/features/workspace/project-workbench/permissions/permissions"

type ConfigDefinitionsPageProps = {
  variant: "workspace" | "project"
}

export function ConfigDefinitionsPage({ variant }: ConfigDefinitionsPageProps) {
  const { t } = useTranslation()
  const me = useMe()

  const canManage = canConfigManage({
    role: me.data?.effective_role,
    scopes: me.data?.token.scopes,
  })

  const title =
    variant === "project"
      ? t("configDefinitions.title")
      : t("configDefinitions.title")
  const description =
    variant === "project"
      ? t("configDefinitions.projectDescription")
      : t("configDefinitions.description")
  const defaultScopes: ConfigAllowedScope[] =
    variant === "project" ? ["project"] : ["workspace"]

  return (
    <div className="space-y-6 p-6">
      <ConfigDefinitionManager
        variant={variant}
        title={title}
        description={description}
        defaultScopes={defaultScopes}
        canManage={canManage}
      />
    </div>
  )
}

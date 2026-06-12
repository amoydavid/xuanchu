import { useTranslation } from "react-i18next"

import type { PageKey } from "@/components/AppShell"
import { resourceConfig } from "@/features/workspace/resources/resource-config"
import { useMe } from "@/features/workspace/session/useMe"
import { ResourcePage } from "@/pages/ResourcePage"

export function ResourceRoute({ page }: { page: PageKey }) {
  const { t } = useTranslation()
  const me = useMe()

  return (
    <ResourcePage
      {...resourceConfig(page, t, me.data?.effective_workspace.slug)}
    />
  )
}

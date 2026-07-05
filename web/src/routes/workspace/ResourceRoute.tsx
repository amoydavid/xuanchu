import { useTranslation } from "react-i18next"

import type { PageKey } from "@/components/AppShell"
import { ResourcePage } from "@/pages/ResourcePage"
import { useMe } from "@/features/workspace/session/useMe"
import { ResourceDispatch } from "@/features/workspace/resources/resource-dispatch"

export function ResourceRoute({ page }: { page: PageKey }) {
  const { t } = useTranslation()
  const me = useMe()

  return (
    <ResourceDispatch
      fallback={ResourcePage}
      page={page}
      t={t}
      workspaceSlug={me.data?.effective_workspace.slug}
    />
  )
}

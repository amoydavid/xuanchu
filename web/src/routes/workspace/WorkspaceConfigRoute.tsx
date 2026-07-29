import { useParams } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import { WorkspaceConfigPage } from "@/pages/workspace-config-page"
import { navigateToDocument } from "@/lib/browser-navigation"

// WorkspaceConfigRoute 渲染 /workspaces/$workspaceSlug/config：
// 轻量面包屑（返回 + workspace slug 标题）+ WorkspaceConfigPage。
export function WorkspaceConfigRoute() {
  const params = useParams({ strict: false }) as { workspaceSlug: string }
  const { t } = useTranslation()
  return (
    <div className="space-y-4">
      <div>
        <Button
          onClick={() => navigateToDocument("/workspaces")}
          size="sm"
          type="button"
          variant="ghost"
        >
          ← {t("page.workspaces")}
        </Button>
        <h1 className="mt-2 text-xl font-semibold tracking-normal">
          <code className="break-all">{params.workspaceSlug}</code>
        </h1>
      </div>
      <WorkspaceConfigPage workspaceSlug={params.workspaceSlug} />
    </div>
  )
}

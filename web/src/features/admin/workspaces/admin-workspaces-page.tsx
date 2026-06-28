import { useQuery } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"
import { useState } from "react"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import { Skeleton } from "@/components/ui/skeleton"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { ApiError } from "@/lib/api"
import { navigateToDocument } from "@/lib/browser-navigation"

import { listAdminWorkspaces } from "./admin-workspace-api"

export function AdminWorkspacesPage() {
  const { t } = useTranslation()
  const [includeArchived, setIncludeArchived] = useState(false)

  const openWorkspace = (slug: string) => {
    // admin 页面是 SPA fallback，直接用浏览器导航即可，
    // 不依赖 TanStack router 的类型化 to，避免跨路由注册耦合。
    navigateToDocument(`/admin/workspaces/${encodeURIComponent(slug)}`)
  }

  const query = useQuery({
    queryKey: ["admin", "workspaces", includeArchived],
    queryFn: () => listAdminWorkspaces(includeArchived),
  })

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h1 className="text-lg font-medium">{t("admin.workspace.title")}</h1>
        <label className="flex items-center gap-2 text-sm text-muted-foreground">
          <Checkbox
            checked={includeArchived}
            onCheckedChange={(v) => setIncludeArchived(v === true)}
          />
          {t("admin.workspace.showArchived")}
        </label>
      </div>

      {query.isLoading ? (
        <div className="space-y-2 rounded-none border bg-card p-3">
          <Skeleton className="h-8 w-full" />
          <Skeleton className="h-8 w-2/3" />
          <Skeleton className="h-8 w-1/2" />
        </div>
      ) : query.isError ? (
        <div className="border bg-card p-4 text-sm text-destructive">
          {errorMessage(query.error, t("common.error"))}
        </div>
      ) : (
        <div className="rounded-none border bg-card">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className="text-muted-foreground">
                  {t("admin.workspace.field.slug")}
                </TableHead>
                <TableHead className="text-muted-foreground">
                  {t("admin.workspace.field.name")}
                </TableHead>
                <TableHead className="text-muted-foreground">
                  {t("admin.workspace.field.visibility")}
                </TableHead>
                <TableHead className="text-muted-foreground">
                  {t("admin.workspace.field.owners")}
                </TableHead>
                <TableHead className="text-muted-foreground">
                  {t("admin.workspace.field.admins")}
                </TableHead>
                <TableHead className="text-muted-foreground">
                  {t("admin.workspace.field.tokens")}
                </TableHead>
                <TableHead className="text-muted-foreground">
                  {t("admin.workspace.field.status")}
                </TableHead>
                <TableHead className="text-muted-foreground text-right">
                  {t("common.actions")}
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {(query.data ?? []).length === 0 ? (
                <TableRow>
                  <TableCell
                    className="h-24 text-center text-muted-foreground"
                    colSpan={8}
                  >
                    {t("common.empty")}
                  </TableCell>
                </TableRow>
              ) : (
                (query.data ?? []).map((row) => {
                  const archived = row.archived_at != null
                  return (
                    <TableRow
                      className={archived ? "opacity-50" : undefined}
                      key={row.id}
                    >
                      <TableCell className="font-medium">{row.slug}</TableCell>
                      <TableCell>{row.name}</TableCell>
                      <TableCell>
                        <Badge variant="outline">
                          {t(`admin.visibility.${row.visibility}`)}
                        </Badge>
                      </TableCell>
                      <TableCell>{row.member_counts.owner}</TableCell>
                      <TableCell>{row.member_counts.admin}</TableCell>
                      <TableCell className="text-xs text-muted-foreground">
                        {t("admin.workspace.tokenSummary", {
                          active: row.token_counts.active,
                          revoked: row.token_counts.revoked,
                          expired: row.token_counts.expired,
                        })}
                      </TableCell>
                      <TableCell>
                        <Badge variant={archived ? "destructive" : "default"}>
                          {archived
                            ? t("admin.workspace.statusArchived")
                            : t("admin.workspace.statusActive")}
                        </Badge>
                      </TableCell>
                      <TableCell className="text-right">
                        <Button
                          onClick={() => openWorkspace(row.slug)}
                          size="sm"
                          variant="ghost"
                        >
                          {t("admin.workspace.open")}
                        </Button>
                      </TableCell>
                    </TableRow>
                  )
                })
              )}
            </TableBody>
          </Table>
        </div>
      )}
    </div>
  )
}

function errorMessage(error: Error | null, fallback: string): string {
  if (error instanceof ApiError) {
    return `${fallback}: ${error.code}`
  }
  return fallback
}

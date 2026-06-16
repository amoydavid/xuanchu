import { useQuery } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"
import { useState } from "react"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Skeleton } from "@/components/ui/skeleton"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { adminApiGet } from "@/features/admin/session/admin-api"
import { deriveTokenStatus } from "@/features/workspace/tokens/token-api"
import { ApiError } from "@/lib/api"

import type { AdminTokenRow } from "./admin-token-api"
import { AdminTokenEditDialog } from "./admin-token-edit-dialog"
import { AdminTokenRevokeDialog } from "./admin-token-revoke-dialog"

export function AdminTokensPage() {
  const { t } = useTranslation()
  const [includeRevoked, setIncludeRevoked] = useState(true)
  const [editTarget, setEditTarget] = useState<AdminTokenRow | null>(null)
  const [revokeTarget, setRevokeTarget] = useState<AdminTokenRow | null>(null)

  const query = useQuery({
    queryKey: ["admin", "tokens", includeRevoked],
    queryFn: () =>
      adminApiGet<AdminTokenRow[]>(
        `/api/v1/admin/tokens?all=${includeRevoked ? "true" : "false"}`
      ),
  })

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h1 className="text-lg font-medium">{t("admin.token.title")}</h1>
        <label className="flex items-center gap-2 text-sm text-muted-foreground">
          <Checkbox
            checked={includeRevoked}
            onCheckedChange={(v) => setIncludeRevoked(v === true)}
          />
          {t("admin.token.showRevoked")}
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
                  {t("token.field.name")}
                </TableHead>
                <TableHead className="text-muted-foreground">
                  {t("token.field.type")}
                </TableHead>
                <TableHead className="text-muted-foreground">
                  {t("common.actor")}
                </TableHead>
                <TableHead className="text-muted-foreground">
                  {t("token.field.workspaces")}
                </TableHead>
                <TableHead className="text-muted-foreground">
                  {t("token.field.scopes")}
                </TableHead>
                <TableHead className="text-muted-foreground">
                  {t("common.status")}
                </TableHead>
                <TableHead className="text-muted-foreground">
                  {t("token.field.lastUsed")}
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
                  const status = deriveTokenStatus(row)
                  const revoked = status === "revoked"
                  return (
                    <TableRow
                      className={revoked ? "opacity-50" : undefined}
                      key={row.id}
                    >
                      <TableCell className="font-medium">{row.name}</TableCell>
                      <TableCell>
                        <Badge variant="outline">{row.type}</Badge>
                      </TableCell>
                      <TableCell>
                        <div className="text-sm">{row.user.name}</div>
                        {row.user.email ? (
                          <div className="text-xs text-muted-foreground">
                            {row.user.email}
                          </div>
                        ) : null}
                      </TableCell>
                      <TableCell className="text-xs text-muted-foreground">
                        {row.workspace_ids.length > 0
                          ? row.workspace_ids.join(", ")
                          : t("admin.token.global")}
                      </TableCell>
                      <TableCell>
                        <Badge variant="secondary">
                          {row.scopes.length} {t("token.field.scopes")}
                        </Badge>
                      </TableCell>
                      <TableCell>
                        <Badge
                          variant={
                            status === "active" ? "default" : "destructive"
                          }
                        >
                          {t(`token.status.${status}`)}
                        </Badge>
                      </TableCell>
                      <TableCell className="text-xs text-muted-foreground">
                        {row.last_used_at ? formatTime(row.last_used_at) : "—"}
                      </TableCell>
                      <TableCell className="text-right">
                        <DropdownMenu>
                          <DropdownMenuTrigger asChild>
                            <Button disabled={revoked} variant="ghost">
                              ⋯
                            </Button>
                          </DropdownMenuTrigger>
                          <DropdownMenuContent align="end">
                            <DropdownMenuItem
                              onClick={() => setEditTarget(row)}
                            >
                              {t("token.edit")}
                            </DropdownMenuItem>
                            <DropdownMenuItem
                              className="text-destructive"
                              onClick={() => setRevokeTarget(row)}
                            >
                              {t("token.revoke")}
                            </DropdownMenuItem>
                          </DropdownMenuContent>
                        </DropdownMenu>
                      </TableCell>
                    </TableRow>
                  )
                })
              )}
            </TableBody>
          </Table>
        </div>
      )}

      {editTarget ? (
        <AdminTokenEditDialog
          onOpenChange={(open) => {
            setEditTarget(open ? editTarget : null)
          }}
          open={editTarget !== null}
          token={editTarget}
        />
      ) : null}
      {revokeTarget ? (
        <AdminTokenRevokeDialog
          onOpenChange={(open) => {
            setRevokeTarget(open ? revokeTarget : null)
          }}
          open={revokeTarget !== null}
          token={revokeTarget}
        />
      ) : null}
    </div>
  )
}

function formatTime(unix: number): string {
  return new Date(unix * 1000).toLocaleString()
}

function errorMessage(error: Error | null, fallback: string): string {
  if (error instanceof ApiError) {
    return `${fallback}: ${error.code}`
  }
  return fallback
}

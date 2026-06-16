import { useQuery } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"
import { useState } from "react"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
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
import { useMe } from "@/features/workspace/session/useMe"
import { workspaceApiGet } from "@/features/workspace/session/workspace-api"
import { ApiError } from "@/lib/api"
import { PageHeader } from "@/pages/OverviewPage"

import { TokenCreateDialog } from "./token-create-dialog"
import { TokenEditDialog } from "./token-edit-dialog"
import { TokenRevokeDialog } from "./token-revoke-dialog"
import { deriveTokenStatus, type TokenRow } from "./token-api"

export function TokensPage() {
  const { t } = useTranslation()
  const me = useMe()
  // queryKey 与原 ResourcePage 一致，复用缓存
  const query = useQuery({
    queryKey: ["resource", "/api/v1/tokens"],
    queryFn: () => workspaceApiGet<TokenRow[]>("/api/v1/tokens"),
  })

  const [createOpen, setCreateOpen] = useState(false)
  const [editTarget, setEditTarget] = useState<TokenRow | null>(null)
  const [revokeTarget, setRevokeTarget] = useState<TokenRow | null>(null)

  // 是否可分配 impersonate scope：对齐后端 tokenManageAllowed（仅 admin/owner）。
  // 用 effective role 判断，而非当前 token 的 scope。
  const role = me.data?.effective_role ?? ""
  const canImpersonate = role === "admin" || role === "owner"

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <PageHeader title={t("token.title")} />
        <Button onClick={() => setCreateOpen(true)} variant="outline">
          {t("token.create")}
        </Button>
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
                  {t("token.field.prefix")}
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
                    colSpan={7}
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
                      <TableCell className="font-mono text-xs text-muted-foreground">
                        {row.prefix}
                      </TableCell>
                      <TableCell>
                        <Badge variant="secondary">
                          {row.scopes.length} {t("token.field.scopes")}
                        </Badge>
                      </TableCell>
                      <TableCell>
                        <Badge
                          variant={
                            status === "active"
                              ? "default"
                              : "destructive"
                          }
                        >
                          {t(`token.status.${status}`)}
                        </Badge>
                      </TableCell>
                      <TableCell className="text-xs text-muted-foreground">
                        {row.last_used_at
                          ? formatTime(row.last_used_at)
                          : "—"}
                      </TableCell>
                      <TableCell className="text-right">
                        <DropdownMenu>
                          <DropdownMenuTrigger asChild>
                            <Button variant="ghost">⋯</Button>
                          </DropdownMenuTrigger>
                          <DropdownMenuContent align="end">
                            <DropdownMenuItem
                              disabled={revoked}
                              onClick={() => setEditTarget(row)}
                            >
                              {t("token.edit")}
                            </DropdownMenuItem>
                            <DropdownMenuItem
                              className="text-destructive"
                              disabled={revoked}
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

      <TokenCreateDialog
        canImpersonate={canImpersonate}
        onOpenChange={setCreateOpen}
        open={createOpen}
      />
      {editTarget ? (
        <TokenEditDialog
          canImpersonate={canImpersonate}
          onOpenChange={(open) => {
            setEditTarget(open ? editTarget : null)
          }}
          open={editTarget !== null}
          token={editTarget}
        />
      ) : null}
      {revokeTarget ? (
        <TokenRevokeDialog
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
  const date = new Date(unix * 1000)
  return date.toLocaleString()
}

function errorMessage(error: Error | null, fallback: string): string {
  if (error instanceof ApiError) {
    return `${fallback}: ${error.code}`
  }
  return fallback
}

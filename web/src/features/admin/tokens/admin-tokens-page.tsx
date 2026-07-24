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
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs"
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

import type { AdminTenantAccessTokenRow, AdminTokenRow } from "./admin-token-api"
import { AdminTenantAccessTokenEditDialog } from "./admin-tenant-access-token-edit-dialog"
import { AdminTenantAccessTokenRevokeDialog } from "./admin-tenant-access-token-revoke-dialog"
import { AdminTokenEditDialog } from "./admin-token-edit-dialog"
import { AdminTokenRevokeDialog } from "./admin-token-revoke-dialog"

type AdminTokenTab = "api" | "tenant"

export function AdminTokensPage() {
  const { t } = useTranslation()
  const [includeRevoked, setIncludeRevoked] = useState(true)
  const [activeTab, setActiveTab] = useState<AdminTokenTab>("api")
  const [editTarget, setEditTarget] = useState<AdminTokenRow | null>(null)
  const [revokeTarget, setRevokeTarget] = useState<AdminTokenRow | null>(null)
  const [tenantEditTarget, setTenantEditTarget] =
    useState<AdminTenantAccessTokenRow | null>(null)
  const [tenantRevokeTarget, setTenantRevokeTarget] =
    useState<AdminTenantAccessTokenRow | null>(null)

  const tokenQuery = useQuery({
    queryKey: ["admin", "tokens", includeRevoked],
    queryFn: () =>
      adminApiGet<AdminTokenRow[]>(
        `/api/v1/admin/tokens?all=${includeRevoked ? "true" : "false"}`
      ),
  })
  const tenantQuery = useQuery({
    queryKey: ["admin", "tenant-access-tokens", includeRevoked],
    queryFn: () =>
      adminApiGet<AdminTenantAccessTokenRow[]>(
        `/api/v1/admin/tenant-access-tokens?all=${includeRevoked ? "true" : "false"}`
      ),
    enabled: activeTab === "tenant",
  })
  const activeQuery = activeTab === "tenant" ? tenantQuery : tokenQuery

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

      <Tabs
        onValueChange={(value) => setActiveTab(value as AdminTokenTab)}
        value={activeTab}
      >
        <TabsList aria-label={t("token.tabs.label")} variant="line">
          <TabsTrigger value="api">{t("token.tabs.api")}</TabsTrigger>
          <TabsTrigger value="tenant">{t("token.tabs.tenant")}</TabsTrigger>
        </TabsList>
      </Tabs>

      {activeQuery.isLoading ? (
        <div className="space-y-2 rounded-none border bg-card p-3">
          <Skeleton className="h-8 w-full" />
          <Skeleton className="h-8 w-2/3" />
          <Skeleton className="h-8 w-1/2" />
        </div>
      ) : activeQuery.isError ? (
        <div className="rounded-lg border bg-card p-4 text-sm text-destructive">
          {errorMessage(activeQuery.error, t("common.error"))}
        </div>
      ) : (
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
                  {activeTab === "tenant"
                    ? t("token.field.prefix")
                    : t("common.actor")}
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
              {(activeQuery.data ?? []).length === 0 ? (
                <TableRow>
                  <TableCell
                    className="h-24 text-center text-muted-foreground"
                    colSpan={8}
                  >
                    {t("common.empty")}
                  </TableCell>
                </TableRow>
              ) : (
                (activeQuery.data ?? []).map((row) => {
                  const status = deriveTokenStatus(row)
                  const revoked = status === "revoked"
                  const tenantRow = row as AdminTenantAccessTokenRow
                  const tokenRow = row as AdminTokenRow
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
                        {activeTab === "tenant" ? (
                          <div className="font-mono text-xs text-muted-foreground">
                            {tenantRow.prefix}
                          </div>
                        ) : (
                          <>
                            <div className="text-sm">{tokenRow.user.name}</div>
                            {tokenRow.user.email ? (
                              <div className="text-xs text-muted-foreground">
                                {tokenRow.user.email}
                              </div>
                            ) : null}
                          </>
                        )}
                      </TableCell>
                      <TableCell className="text-xs text-muted-foreground">
                        {activeTab === "tenant"
                          ? tenantRow.workspace_id
                          : (tokenRow.workspace_ids ?? []).length > 0
                            ? (tokenRow.workspace_ids ?? []).join(", ")
                            : t("admin.token.global")}
                      </TableCell>
                      <TableCell>
                        <Badge variant="secondary">
                          {(row.scopes ?? []).length} {t("token.field.scopes")}
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
                              onClick={() => {
                                if (activeTab === "tenant") {
                                  setTenantEditTarget(tenantRow)
                                  return
                                }
                                setEditTarget(tokenRow)
                              }}
                            >
                              {t("token.edit")}
                            </DropdownMenuItem>
                            <DropdownMenuItem
                              className="text-destructive"
                              onClick={() => {
                                if (activeTab === "tenant") {
                                  setTenantRevokeTarget(tenantRow)
                                  return
                                }
                                setRevokeTarget(tokenRow)
                              }}
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
      {tenantEditTarget ? (
        <AdminTenantAccessTokenEditDialog
          onOpenChange={(open) => {
            setTenantEditTarget(open ? tenantEditTarget : null)
          }}
          open={tenantEditTarget !== null}
          token={tenantEditTarget}
        />
      ) : null}
      {tenantRevokeTarget ? (
        <AdminTenantAccessTokenRevokeDialog
          onOpenChange={(open) => {
            setTenantRevokeTarget(open ? tenantRevokeTarget : null)
          }}
          open={tenantRevokeTarget !== null}
          token={tenantRevokeTarget}
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

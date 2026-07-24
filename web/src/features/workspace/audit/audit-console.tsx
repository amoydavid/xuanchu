import { useQuery } from "@tanstack/react-query"
import { useMemo, useState } from "react"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { workspaceApiGet } from "@/features/workspace/session/workspace-api"
import { ApiError } from "@/lib/api"

import { auditPath, type AuditRow } from "./audit-api"
import { filterAuditRows, rowsToCSV, type AuditFilter } from "./audit-filter"

export function AuditConsole({ workspaceSlug }: { workspaceSlug?: string }) {
  const { t } = useTranslation()
  const [project, setProject] = useState<string>("")
  const [actor, setActor] = useState<string>("")
  const [action, setAction] = useState<string>("")
  const [target, setTarget] = useState<string>("")

  const path = useMemo(
    () => auditPath({ project: project || undefined, limit: 100 }),
    [project]
  )

  const query = useQuery<AuditRow[]>({
    queryKey: ["audit", workspaceSlug, path],
    queryFn: () => workspaceApiGet<AuditRow[]>(path),
  })

  const filter: AuditFilter = useMemo(
    () => ({
      actor: actor || undefined,
      action: action || undefined,
      target: target || undefined,
    }),
    [actor, action, target]
  )

  const rows = query.data ? filterAuditRows(query.data, filter) : []

  function handleExport() {
    const csv = rowsToCSV(rows)
    const blob = new Blob([csv], { type: "text/csv;charset=utf-8" })
    const url = URL.createObjectURL(blob)
    const anchor = document.createElement("a")
    anchor.href = url
    anchor.download = `audit-${new Date().toISOString().slice(0, 10)}.csv`
    document.body.appendChild(anchor)
    anchor.click()
    document.body.removeChild(anchor)
    URL.revokeObjectURL(url)
  }

  return (
    <div className="space-y-4">
      <div className="flex items-start justify-between gap-4">
        <div>
          <h1 className="text-xl font-semibold tracking-normal">
            {t("page.audit")}
          </h1>
          <p className="mt-1 text-xs text-muted-foreground">
            {t("audit.currentResultHint")}
          </p>
        </div>
        <Button onClick={handleExport} size="sm" variant="outline">
          {t("audit.export")}
        </Button>
      </div>

      <div className="rounded-lg flex flex-wrap items-center gap-2 border bg-card p-2">
        <Input
          aria-label={t("audit.searchPlaceholder")}
          className="h-8 max-w-xs"
          onChange={(e) => setActor(e.target.value)}
          placeholder={t("audit.searchPlaceholder")}
          value={actor}
        />
        <Input
          aria-label={t("common.actions")}
          className="h-8 w-32"
          onChange={(e) => setAction(e.target.value)}
          placeholder={t("common.actions")}
          value={action}
        />
        <Input
          aria-label={t("overview.target")}
          className="h-8 w-32"
          onChange={(e) => setTarget(e.target.value)}
          placeholder={t("overview.target")}
          value={target}
        />
        <Input
          aria-label={t("projectReadonly.project")}
          className="h-8 w-32"
          onChange={(e) => setProject(e.target.value)}
          placeholder={t("projectReadonly.project")}
          value={project}
        />
        <span className="text-[11px] text-muted-foreground">
          {t("audit.currentResultOnly")}
        </span>
      </div>

      {query.isError ? (
        <div className="rounded-lg border bg-card p-4 text-sm text-destructive">
          {query.error instanceof ApiError ? query.error.message : t("common.error")}
        </div>
      ) : query.isLoading ? (
        <div className="rounded-lg border bg-card p-6 text-sm text-muted-foreground">
          {t("common.loading")}
        </div>
      ) : rows.length === 0 ? (
        <div className="rounded-lg border bg-card p-6 text-sm text-muted-foreground">
          {t("common.empty")}
        </div>
      ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t("audit.time")}</TableHead>
                <TableHead>{t("common.actor")}</TableHead>
                <TableHead>{t("overview.action")}</TableHead>
                <TableHead>{t("overview.target")}</TableHead>
                <TableHead>{t("audit.targetId")}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {rows.map((row) => (
                <TableRow key={row.id}>
                  <TableCell className="whitespace-nowrap text-xs text-muted-foreground">
                    {formatTime(row.created_at)}
                  </TableCell>
                  <TableCell className="truncate">
                    {row.actor?.display_name || row.actor?.name || row.actor?.id || "-"}
                  </TableCell>
                  <TableCell className="truncate">{row.action}</TableCell>
                  <TableCell>{row.target_type}</TableCell>
                  <TableCell className="truncate">{row.target_id}</TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
      )}
    </div>
  )
}

function formatTime(ts: number): string {
  const d = new Date(ts * 1000)
  const y = d.getFullYear()
  const m = String(d.getMonth() + 1).padStart(2, "0")
  const day = String(d.getDate()).padStart(2, "0")
  const hh = String(d.getHours()).padStart(2, "0")
  const mm = String(d.getMinutes()).padStart(2, "0")
  return `${y}-${m}-${day} ${hh}:${mm}`
}

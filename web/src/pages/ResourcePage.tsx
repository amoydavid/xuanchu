import { useQuery } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"

import { DataTable, type Column } from "@/components/DataTable"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { workspaceApiGet } from "@/features/workspace/session/workspace-api"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import { ApiError } from "@/lib/api"
import { PageHeader } from "./OverviewPage"

type Row = Record<string, unknown>

type ResourcePageProps = {
  columns: Column<Row>[]
  enabled?: boolean
  path: string
  title: string
}

export function ResourcePage({
  columns,
  enabled = true,
  path,
  title,
}: ResourcePageProps) {
  const { t } = useTranslation()
  const query = useQuery({
    enabled,
    queryKey: ["resource", path],
    queryFn: () => workspaceApiGet<unknown>(path),
  })

  return (
    <div className="space-y-4">
      <PageHeader title={title} />
      <div className="flex flex-wrap gap-2 border bg-card p-2">
        <Input className="flex-1" placeholder={t("common.search")} />
        <Button variant="outline">{t("common.filter")}</Button>
      </div>
      {query.isLoading || !enabled ? (
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
        <DataTable
          columns={columns}
          empty={t("common.empty")}
          rows={normalizeRows(query.data)}
        />
      )}
    </div>
  )
}

function normalizeRows(data: unknown): Row[] {
  if (Array.isArray(data)) {
    return data.filter(isRow)
  }
  if (isRow(data)) {
    return Object.entries(data).map(([key, value]) => ({ key, value }))
  }
  return []
}

function isRow(value: unknown): value is Row {
  return Boolean(value) && typeof value === "object" && !Array.isArray(value)
}

export function textCell(key: string): (row: Row) => string {
  return (row) => String(row[key] ?? "")
}

export function statusCell(key: string): (row: Row) => React.ReactNode {
  return (row) => <Badge variant="outline">{String(row[key] ?? "")}</Badge>
}

export function userCell(key: string): (row: Row) => string {
  return (row) => {
    const value = row[key]
    if (value && typeof value === "object" && "name" in value) {
      return String(value.name ?? "")
    }
    return String(value ?? "")
  }
}

function errorMessage(error: Error | null, fallback: string): string {
  if (error instanceof ApiError) {
    return `${fallback}: ${error.code}`
  }
  return fallback
}

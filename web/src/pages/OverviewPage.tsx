import { useMemo, useState } from "react"
import type React from "react"
import { Link } from "@tanstack/react-router"
import { useQuery, useQueryClient } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"

import { useBrandName } from "@/brand/BrandContext"
import { DataTable } from "@/components/DataTable"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { listWorkspaceEffectiveConfig } from "@/features/workspace/config/config-definition-api"
import type { ConfigEffectiveValue } from "@/features/workspace/config/config-definition-api"
import { workspaceApiGet } from "@/features/workspace/session/workspace-api"
import type { MeResponse } from "@/features/workspace/session/useMe"
import { ApiError } from "@/lib/api"

type Row = Record<string, unknown>

type OverviewPageProps = {
  me?: MeResponse
}

export function OverviewPage({ me }: OverviewPageProps) {
  const { t } = useTranslation()
  const brandName = useBrandName()
  const queryClient = useQueryClient()
  const tasks = useQuery({
    queryKey: ["overview", "tasks"],
    queryFn: () => workspaceApiGet<Row[]>("/api/v1/tasks?limit=200"),
  })
  const projects = useQuery({
    queryKey: ["overview", "projects"],
    queryFn: () => workspaceApiGet<Row[]>("/api/v1/projects"),
  })
  const deadLettered = useQuery({
    queryKey: ["overview", "notification-deliveries", "dead_lettered"],
    queryFn: () =>
      workspaceApiGet<Row[]>(
        "/api/v1/notification-deliveries?limit=5&status=dead_lettered"
      ),
  })
  const retryWait = useQuery({
    queryKey: ["overview", "notification-deliveries", "retry_wait"],
    queryFn: () =>
      workspaceApiGet<Row[]>(
        "/api/v1/notification-deliveries?limit=5&status=retry_wait"
      ),
  })
  const audit = useQuery({
    queryKey: ["overview", "audit"],
    queryFn: () => workspaceApiGet<Row[]>("/api/v1/audit?limit=5"),
  })
  const homeConfig = useQuery({
    queryKey: ["overview", "config-effective", "console-home"],
    queryFn: () => listWorkspaceEffectiveConfig({ consoleHome: true }),
  })

  const deliveries = useMemo(
    () =>
      [...(deadLettered.data ?? []), ...(retryWait.data ?? [])]
        .sort(
          (left, right) =>
            numericField(right, "created_at") - numericField(left, "created_at")
        )
        .slice(0, 5),
    [deadLettered.data, retryWait.data]
  )
  const [now] = useState(() => Math.floor(Date.now() / 1000))
  const soon = now + 7 * 24 * 60 * 60
  const taskRows = tasks.data ?? []
  const metrics = [
    [t("overview.tasks"), valueOrDash(tasks, taskRows.length)],
    [t("overview.overdue"), valueOrDash(tasks, countDueBefore(taskRows, now))],
    [
      t("overview.dueSoon"),
      valueOrDash(tasks, countDueBetween(taskRows, now, soon)),
    ],
    [t("overview.projects"), valueOrDash(projects, projects.data?.length ?? 0)],
    [
      t("overview.failedDelivery"),
      deadLettered.isPending || retryWait.isPending ? "-" : deliveries.length,
    ],
  ] as const

  return (
    <div className="space-y-5">
      <PageHeader
        title={t("page.overview")}
        description={t("app.description", { brand: brandName })}
      />
      <section className="grid gap-3 md:grid-cols-3">
        {[
          [t("overview.currentActor"), me?.actor.name ?? "-"],
          [t("overview.tokenType"), me?.token.type ?? "-"],
          [t("overview.scope"), (me?.token.scopes ?? []).join(", ") || "-"],
        ].map(([label, value]) => (
          <div className="border bg-card p-3" key={label}>
            <div className="text-xs text-muted-foreground">{label}</div>
            <div className="mt-1 text-sm font-medium">{value}</div>
          </div>
        ))}
      </section>
      <section className="grid gap-3 md:grid-cols-5">
        {metrics.map(([label, value]) => (
          <div className="border bg-card p-3" key={label}>
            <div className="text-xs text-muted-foreground">{label}</div>
            <div className="mt-1 text-lg font-semibold">{value}</div>
          </div>
        ))}
      </section>
      <ConfigOverviewSection rows={homeConfig.data ?? []} isPending={homeConfig.isPending} isError={homeConfig.isError} />
      <QueryError error={tasks.error ?? projects.error} />
      <section className="space-y-2">
        <SectionTitle
          action={
            <Button
              onClick={() =>
                void queryClient.invalidateQueries({ queryKey: ["overview"] })
              }
              variant="outline"
            >
              {t("common.refresh")}
            </Button>
          }
          title={t("overview.recentFailedDeliveries")}
        />
        {deadLettered.isPending || retryWait.isPending ? (
          <TableSkeleton />
        ) : deadLettered.isError || retryWait.isError ? (
          <QueryError error={deadLettered.error ?? retryWait.error} />
        ) : (
          <DataTable
            columns={[
              {
                key: "id",
                header: t("overview.delivery"),
                render: (row) => (
                  <code className="break-all">{textValue(row.id)}</code>
                ),
              },
              {
                key: "event_type",
                header: t("overview.kind"),
                render: (row) => textValue(row.event_type),
              },
              {
                key: "sink_id",
                header: t("overview.sink"),
                render: (row) => textValue(row.sink_id),
              },
              {
                key: "status",
                header: t("common.status"),
                render: (row) => (
                  <Badge variant="outline">{textValue(row.status)}</Badge>
                ),
              },
            ]}
            empty={t("common.empty")}
            rows={deliveries}
          />
        )}
      </section>
      <section className="space-y-2">
        <SectionTitle title={t("overview.recentAudit")} />
        {audit.isPending ? (
          <TableSkeleton />
        ) : audit.isError ? (
          <QueryError error={audit.error} />
        ) : (
          <DataTable
            columns={[
              {
                key: "created_at",
                header: t("overview.time"),
                render: (row) => formatUnix(row.created_at),
              },
              {
                key: "actor",
                header: t("common.actor"),
                render: (row) => userName(row.actor),
              },
              {
                key: "action",
                header: t("overview.action"),
                render: (row) => textValue(row.action),
              },
              {
                key: "target_type",
                header: t("overview.target"),
                render: (row) => textValue(row.target_type),
              },
            ]}
            empty={t("common.empty")}
            rows={audit.data ?? []}
          />
        )}
      </section>
    </div>
  )
}

export function PageHeader({
  description,
  title,
}: {
  description?: string
  title: string
}) {
  return (
    <div className="flex items-start justify-between gap-4">
      <div>
        <h1 className="text-xl font-semibold tracking-normal">{title}</h1>
        {description ? (
          <p className="mt-1 text-sm text-muted-foreground">{description}</p>
        ) : null}
      </div>
    </div>
  )
}

function SectionTitle({
  action,
  title,
}: {
  action?: React.ReactNode
  title: string
}) {
  return (
    <div className="flex h-8 items-center justify-between">
      <h2 className="text-sm font-medium">{title}</h2>
      {action}
    </div>
  )
}

function TableSkeleton() {
  return (
    <div className="space-y-2 rounded-none border bg-card p-3">
      <Skeleton className="h-8 w-full" />
      <Skeleton className="h-8 w-2/3" />
      <Skeleton className="h-8 w-1/2" />
    </div>
  )
}

function QueryError({ error }: { error?: Error | null }) {
  const { t } = useTranslation()
  if (!error) {
    return null
  }
  const code = error instanceof ApiError ? error.code : "unknown"
  return (
    <div className="border bg-card p-4 text-sm text-destructive">
      {t("common.error")}: {code}
    </div>
  )
}

function valueOrDash(
  query: { isPending: boolean },
  value: number
): number | string {
  return query.isPending ? "-" : value
}

function countDueBefore(rows: Row[], timestamp: number): number {
  return rows.filter((row) => {
    const due = numericField(row, "due")
    return due > 0 && due < timestamp && textValue(row.status) !== "completed"
  }).length
}

function countDueBetween(rows: Row[], from: number, to: number): number {
  return rows.filter((row) => {
    const due = numericField(row, "due")
    return due >= from && due <= to && textValue(row.status) !== "completed"
  }).length
}

function numericField(row: Row, key: string): number {
  const value = row[key]
  if (typeof value === "number") {
    return value
  }
  if (typeof value === "string") {
    const parsed = Number(value)
    return Number.isFinite(parsed) ? parsed : 0
  }
  return 0
}

function textValue(value: unknown): string {
  if (value === null || value === undefined) {
    return ""
  }
  return String(value)
}

function userName(value: unknown): string {
  if (value && typeof value === "object" && "name" in value) {
    return textValue(value.name)
  }
  return textValue(value)
}

function formatUnix(value: unknown): string {
  const timestamp = typeof value === "number" ? value : Number(value)
  if (!Number.isFinite(timestamp) || timestamp <= 0) {
    return ""
  }
  return new Date(timestamp * 1000).toLocaleString()
}

function ConfigOverviewSection({
  rows,
  isPending,
  isError,
}: {
  rows: ConfigEffectiveValue[]
  isPending: boolean
  isError: boolean
}) {
  const { t } = useTranslation()
  return (
    <section className="space-y-2">
      <SectionTitle
        action={
          <Link
            className="text-sm text-primary hover:underline"
            to="/settings"
          >
            {t("configDefinitions.overviewGoToDefinitions")}
          </Link>
        }
        title={t("configDefinitions.overviewTitle")}
      />
      {isPending ? (
        <TableSkeleton />
      ) : isError ? (
        <div className="border bg-card p-3 text-sm text-destructive">
          {t("common.error")}
        </div>
      ) : rows.length === 0 ? (
        <div className="border bg-card p-3 text-sm text-muted-foreground">
          {t("configDefinitions.overviewEmpty")}
        </div>
      ) : (
        <div className="divide-y rounded-lg border bg-card">
          {rows.map((row) => {
            const primary = row.definition.label || row.key
            const sourceText =
              row.source === "project"
                ? t("configDefinitions.sourceProject")
                : row.source === "workspace"
                  ? t("configDefinitions.sourceWorkspace")
                  : row.source === "default"
                    ? t("configDefinitions.sourceDefault")
                    : t("configDefinitions.sourceMissing")
            const displayValue =
              row.value === null ? t("configDefinitions.sourceMissing") : row.value
            return (
              <div className="flex items-center gap-3 p-3" key={row.key}>
                <div className="min-w-0 flex-1">
                  <div className="text-sm font-medium">{primary}</div>
                  <code className="text-xs text-muted-foreground">
                    {row.key}
                  </code>
                </div>
                <div className="max-w-[40%] truncate text-sm text-muted-foreground">
                  {displayValue}
                </div>
                <Badge variant="outline">{sourceText}</Badge>
              </div>
            )
          })}
        </div>
      )}
    </section>
  )
}
